package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"strings"
)

const encryptedContextPrefixLimit = 64 << 10

type encryptedContextRecovery struct {
	request map[string]json.RawMessage
	reason  string
	removed int
}

func hasEncryptedReasoning(request map[string]json.RawMessage) bool {
	var items []struct {
		Type             string          `json:"type"`
		EncryptedContent json.RawMessage `json:"encrypted_content"`
	}
	if json.Unmarshal(request["input"], &items) != nil {
		return false
	}
	for _, item := range items {
		if item.Type == "reasoning" && len(item.EncryptedContent) > 0 && string(item.EncryptedContent) != "null" {
			return true
		}
	}
	return false
}

// Only code-owned labels escape this function, never upstream messages or IDs.
// Do not search arbitrary output text: only explicit error envelopes qualify.
func encryptedContextError(data []byte) string {
	var envelope struct {
		Type     string            `json:"type"`
		Code     string            `json:"code"`
		Message  string            `json:"message"`
		Error    json.RawMessage   `json:"error"`
		Output   []json.RawMessage `json:"output"`
		Response struct {
			Error  json.RawMessage   `json:"error"`
			Output []json.RawMessage `json:"output"`
		} `json:"response"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return ""
	}
	if len(envelope.Output) != 0 || len(envelope.Response.Output) != 0 {
		return ""
	}
	code, message := envelope.Code, envelope.Message
	if len(envelope.Response.Error) > 0 && envelope.Type == "response.failed" {
		envelope.Error = envelope.Response.Error
	}
	if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		var detail struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(envelope.Error, &detail) != nil {
			return ""
		}
		code, message = detail.Code, detail.Message
	} else if envelope.Type != "error" && envelope.Type != "invalid_request_error" {
		return ""
	}
	if code == "invalid_encrypted_content" {
		return "invalid_encrypted_content"
	}
	if strings.HasPrefix(message, "The encrypted content") && strings.Contains(message, "could not be verified") {
		return "encrypted_content_unverifiable"
	}
	if strings.Contains(message, "was not issued to this caller") && (strings.Contains(message, "encrypted_content") || strings.Contains(message, "reasoning")) {
		return "encrypted_content_caller_mismatch"
	}
	return ""
}

// Drop all historical reasoning only after a definite rejection; the error ID
// does not establish which other items belong to the same encryption domain.
// Keep messages, calls/results, compaction and encrypted agent tasks untouched.
func recoverEncryptedContext(request map[string]json.RawMessage, reason string) *encryptedContextRecovery {
	if reason == "" {
		return nil
	}
	var items []json.RawMessage
	if json.Unmarshal(request["input"], &items) != nil {
		return nil
	}
	kept := make([]json.RawMessage, 0, len(items))
	removed := 0
	for _, raw := range items {
		var item struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &item) == nil && item.Type == "reasoning" {
			removed++
			continue
		}
		kept = append(kept, raw)
	}
	if removed == 0 {
		return nil
	}
	cleaned := maps.Clone(request)
	cleaned["input"], _ = json.Marshal(kept)
	return &encryptedContextRecovery{request: cleaned, reason: reason, removed: removed}
}

type encryptedContextRead struct {
	data []byte
	err  error
}

// At most one upstream Read is outstanding. On request cancellation, hand
// that read back to the stream instead of abandoning it.
// Closing the HTTP response body/canceling its context unblocks the read.
type encryptedContextPendingRead struct {
	result <-chan encryptedContextRead
	data   *bytes.Reader
	err    error
}

func (p *encryptedContextPendingRead) Read(b []byte) (int, error) {
	if p.data == nil {
		r := <-p.result
		p.data, p.err = bytes.NewReader(r.data), r.err
	}
	if p.data.Len() > 0 {
		return p.data.Read(b)
	}
	if p.err != nil {
		return 0, p.err
	}
	return 0, io.EOF
}

// Hold only lifecycle events, for at most 64 KiB and within the existing
// upstream request deadline. A slow validation error is still recoverable
// before output; an independent short timer would commit it prematurely.
// The first non-lifecycle event commits the stream, even an empty tool/reasoning shell.
// Preserve every inspected byte when declining recovery. Never replay after
// generated output, EOF, timeout, malformed data, or an unrelated stream error.
func primeEncryptedContextStream(ctx context.Context, body io.Reader) (io.Reader, string, string) {
	prefix := make([]byte, 0, 4096)
	lineStart := 0
	var data strings.Builder
	event := ""
	replay := func(tail io.Reader) io.Reader { return io.MultiReader(bytes.NewReader(prefix), tail) }
	pass := func(tail io.Reader, inspection string) (io.Reader, string, string) {
		return replay(tail), "", inspection
	}
	for len(prefix) < encryptedContextPrefixLimit {
		chunkSize := min(4096, encryptedContextPrefixLimit-len(prefix))
		result := make(chan encryptedContextRead, 1)
		go func() {
			buf := make([]byte, chunkSize)
			n, err := body.Read(buf)
			result <- encryptedContextRead{data: buf[:n], err: err}
		}()
		var chunk encryptedContextRead
		select {
		case chunk = <-result:
		case <-ctx.Done():
			inspection := "cancelled"
			if ctx.Err() == context.DeadlineExceeded {
				inspection = "timeout"
			}
			return pass(io.MultiReader(&encryptedContextPendingRead{result: result}, body), inspection)
		}
		prefix = append(prefix, chunk.data...)
		for {
			newline := bytes.IndexByte(prefix[lineStart:], '\n')
			if newline < 0 {
				break
			}
			end := lineStart + newline
			line := strings.TrimSuffix(string(prefix[lineStart:end]), "\r")
			lineStart = end + 1
			if line == "" {
				if data.Len() == 0 {
					event = ""
					continue
				}
				payload := []byte(data.String())
				var header struct {
					Type     string `json:"type"`
					Response struct {
						Output []json.RawMessage `json:"output"`
					} `json:"response"`
				}
				if json.Unmarshal(payload, &header) != nil {
					return pass(body, "invalid_event")
				}
				if header.Type == "" && event == "error" {
					// Some SSE errors put type only in the event line.
					var fields map[string]json.RawMessage
					if json.Unmarshal(payload, &fields) == nil && fields != nil {
						fields["type"] = json.RawMessage(`"error"`)
						payload, _ = json.Marshal(fields)
					}
				}
				if header.Type == "error" || header.Type == "response.failed" || event == "error" {
					if reason := encryptedContextError(payload); reason != "" {
						return replay(body), reason, "rejected"
					}
				}
				switch header.Type {
				case "response.created", "response.in_progress", "response.queued":
					if len(header.Response.Output) != 0 {
						return pass(body, "output_started")
					}
				default:
					return pass(body, "non_lifecycle_event")
				}
				data.Reset()
				event = ""
			} else if strings.HasPrefix(line, "data:") {
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(strings.TrimPrefix(line[5:], " "))
			} else if strings.HasPrefix(line, "event:") {
				event = strings.TrimPrefix(line[6:], " ")
			}
		}
		if chunk.err != nil {
			// A failed read is not a definite upstream rejection.
			inspection := "read_error"
			switch {
			case ctx.Err() == context.DeadlineExceeded:
				inspection = "timeout"
			case ctx.Err() != nil:
				inspection = "cancelled"
			case chunk.err == io.EOF:
				inspection = "eof"
			}
			return pass(&encryptedContextPendingRead{data: bytes.NewReader(nil), err: chunk.err}, inspection)
		}
		if len(chunk.data) == 0 {
			return pass(body, "empty_read")
		}
	}
	return pass(body, "prefix_limit")
}
