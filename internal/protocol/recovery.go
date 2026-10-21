package protocol

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"strings"
)

// Response initialization can echo large instruction/tool schemas. Match the
// existing protocol SSE event bound instead of committing at just 64 KiB.
const RecoveryPrefixLimit = 2 << 20

type Recovery struct {
	Request map[string]json.RawMessage
	Reason  string
	Removed int
	session *Session
}

func HasEncryptedReasoning(request map[string]json.RawMessage) bool {
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
func EncryptedContextError(data []byte) string {
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
	if encryptedContextHasOutput(envelope.Output) || encryptedContextHasOutput(envelope.Response.Output) {
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
	if strings.Contains(message, "The encrypted content") && strings.Contains(message, "could not be verified") {
		return "encrypted_content_unverifiable"
	}
	if strings.Contains(message, "was not issued to this caller") && (strings.Contains(message, "encrypted_content") || strings.Contains(message, "reasoning")) {
		return "encrypted_content_caller_mismatch"
	}
	return ""
}

// An empty message/reasoning item announces a container, not generated content
// or a tool invocation. Some upstreams emit these before validating history.
// Unknown fields and item kinds remain a commit boundary.
func encryptedContextEmptyItem(raw json.RawMessage) bool {
	var item map[string]json.RawMessage
	if json.Unmarshal(raw, &item) != nil || item == nil {
		return false
	}
	var kind, status string
	if json.Unmarshal(item["type"], &kind) != nil || (kind != "reasoning" && kind != "message") {
		return false
	}
	for name, value := range item {
		switch name {
		case "type", "id", "role":
			var s string
			if json.Unmarshal(value, &s) != nil {
				return false
			}
		case "status":
			if json.Unmarshal(value, &status) != nil || (status != "" && status != "in_progress") {
				return false
			}
		case "summary", "content":
			var parts []json.RawMessage
			if json.Unmarshal(value, &parts) != nil || len(parts) != 0 {
				return false
			}
		case "encrypted_content":
			if string(value) != "null" && string(value) != `""` {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func encryptedContextHasOutput(items []json.RawMessage) bool {
	for _, item := range items {
		if !encryptedContextEmptyItem(item) {
			return true
		}
	}
	return false
}

func encryptedContextEmptyPart(raw json.RawMessage) bool {
	var part map[string]json.RawMessage
	if json.Unmarshal(raw, &part) != nil || part == nil {
		return false
	}
	var kind string
	if json.Unmarshal(part["type"], &kind) != nil || (kind != "summary_text" && kind != "output_text" && kind != "refusal") {
		return false
	}
	for name, value := range part {
		switch name {
		case "type":
		case "text", "refusal":
			var s string
			if json.Unmarshal(value, &s) != nil || s != "" {
				return false
			}
		case "annotations", "logprobs":
			var values []json.RawMessage
			if json.Unmarshal(value, &values) != nil || len(values) != 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Only these code-owned categories are logged, never raw event names or items.
func encryptedContextEventLabel(kind string, item json.RawMessage) string {
	switch kind {
	case "response.created", "response.in_progress", "response.queued":
		return "lifecycle"
	case "codex.response.metadata":
		return "metadata"
	case "error", "response.failed":
		return "error"
	case "response.output_item.added", "response.output_item.done":
		var header struct {
			Type string `json:"type"`
		}
		json.Unmarshal(item, &header)
		switch header.Type {
		case "reasoning":
			return "reasoning_item"
		case "message":
			return "message_item"
		case "function_call", "custom_tool_call":
			return "tool_item"
		default:
			return "other_item"
		}
	case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
		return "reasoning_delta"
	case "response.output_text.delta", "response.refusal.delta":
		return "text_delta"
	case "response.reasoning_summary_part.added", "response.content_part.added":
		return "content_part"
	case "response.completed", "response.incomplete":
		return "terminal"
	default:
		return "unknown"
	}
}

// Drop all historical reasoning only after a definite rejection; the error ID
// does not establish which other items belong to the same encryption domain.
// Keep messages, calls/results, compaction and encrypted agent tasks untouched.
func RecoverEncryptedContext(request map[string]json.RawMessage, reason string) *Recovery {
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
	return &Recovery{Request: cleaned, Reason: reason, Removed: removed}
}

type encryptedContextRead struct {
	data []byte
	err  error
}

type RecoveryStreamDetails struct {
	TypeHash string
	Fields   string
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

// Hold pre-output announcements, for at most 2 MiB and within the existing
// upstream request deadline. A slow validation error is still recoverable
// before output; an independent short timer would commit it prematurely.
// Empty message/reasoning shells and empty content-part announcements are also
// held. Actual content, tool items and unknown events commit the stream.
// Preserve every inspected byte when declining recovery. Never replay after
// generated output, EOF, timeout, malformed data, or an unrelated stream error.
func PrimeRecoveryStream(ctx context.Context, body io.Reader, details ...*RecoveryStreamDetails) (io.Reader, string, string, string) {
	prefix := make([]byte, 0, 4096)
	lineStart := 0
	var data strings.Builder
	event := ""
	lastEvent := "none"
	replay := func(tail io.Reader) io.Reader { return io.MultiReader(bytes.NewReader(prefix), tail) }
	pass := func(tail io.Reader, inspection string) (io.Reader, string, string, string) {
		return replay(tail), "", inspection, lastEvent
	}
	for len(prefix) < RecoveryPrefixLimit {
		chunkSize := min(4096, RecoveryPrefixLimit-len(prefix))
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
					Type     string          `json:"type"`
					Item     json.RawMessage `json:"item"`
					Part     json.RawMessage `json:"part"`
					Response struct {
						Output []json.RawMessage `json:"output"`
					} `json:"response"`
				}
				if json.Unmarshal(payload, &header) != nil {
					return pass(body, "invalid_event")
				}
				if header.Type == "" && event != "" {
					// SSE may carry the event type only in its event field.
					header.Type = event
					var fields map[string]json.RawMessage
					if json.Unmarshal(payload, &fields) == nil && fields != nil {
						fields["type"], _ = json.Marshal(event)
						payload, _ = json.Marshal(fields)
					}
				}
				lastEvent = encryptedContextEventLabel(header.Type, header.Item)
				for _, detail := range details {
					if detail == nil {
						continue
					}
					detail.TypeHash = fmt.Sprintf("%x", sha256.Sum256([]byte(header.Type)))
					var fields map[string]json.RawMessage
					json.Unmarshal(payload, &fields)
					var present []string
					for _, name := range []string{"type", "event", "response", "item", "part", "error", "data", "object", "id", "choices", "message", "metadata"} {
						if _, ok := fields[name]; ok {
							present = append(present, name)
						}
					}
					detail.Fields = strings.Join(present, ",")
					if detail.Fields == "" {
						detail.Fields = "none"
					}
				}
				if header.Type == "error" || header.Type == "response.failed" || event == "error" {
					if reason := EncryptedContextError(payload); reason != "" {
						return replay(body), reason, "rejected", lastEvent
					}
				}
				switch header.Type {
				case "response.created", "response.in_progress", "response.queued", "codex.response.metadata":
					if encryptedContextHasOutput(header.Response.Output) {
						return pass(body, "output_started")
					}
				case "response.output_item.added":
					if !encryptedContextEmptyItem(header.Item) {
						return pass(body, "non_lifecycle_event")
					}
				case "response.reasoning_summary_part.added", "response.content_part.added":
					if !encryptedContextEmptyPart(header.Part) {
						return pass(body, "non_lifecycle_event")
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

// CanRecover permits the existing one-shot native Responses recovery only.
// The gateway decides whether the HTTP status and replay budget allow it.
func (s *Session) CanRecover(upstream string) bool {
	return s.client == "responses" && upstream == "responses" && HasEncryptedReasoning(s.PolicyFields())
}
func (s *Session) RecoverError(data []byte) *Recovery { return s.Recover(EncryptedContextError(data)) }
func (s *Session) Recover(reason string) *Recovery {
	if !s.CanRecover("responses") {
		return nil
	}
	switch reason {
	case "invalid_encrypted_content", "encrypted_content_unverifiable", "encrypted_content_caller_mismatch":
	default:
		return nil
	}
	recovery := RecoverEncryptedContext(s.PolicyFields(), reason)
	if recovery != nil {
		raw, _ := json.Marshal(recovery.Request)
		recovery.session, _ = NewSession(raw, s.client)
	}
	return recovery
}
func (r *Recovery) Session() *Session { return r.session }
