package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

var sessionRequests = map[string]string{
	"responses":        `{"model":"public","input":"hello","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`,
	"chat-completions": `{"model":"public","messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}`,
	"anthropic":        `{"model":"public","max_tokens":20,"messages":[{"role":"user","content":"hello"}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`,
}
var sessionReplies = map[string]string{
	"responses":        `{"id":"r","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":0,"output_tokens":0}}`,
	"chat-completions": `{"id":"r","choices":[{"message":{"content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0}}`,
	"anthropic":        `{"id":"r","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":0,"output_tokens":0}}`,
}
var sessionStreams = map[string]string{
	"responses":        sse(`{"type":"response.created","response":{"id":"r"}}`, `{"type":"response.output_item.added","output_index":0,"item":{"type":"message"}}`, `{"type":"response.output_text.delta","output_index":0,"delta":"hello"}`, `{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":0,"output_tokens":0}}}`),
	"chat-completions": sse(`{"id":"r","choices":[{"index":0,"delta":{"content":"hello"}}]}`, `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0}}`, `[DONE]`),
	"anthropic":        sse(`{"type":"message_start","message":{"id":"r","usage":{"input_tokens":0,"output_tokens":0}}}`, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`, `{"type":"content_block_stop","index":0}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`, `{"type":"message_stop"}`),
}

func newTestSession(t *testing.T, raw, from string) *Session {
	t.Helper()
	s, err := NewSession([]byte(raw), from)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func prepareTestAttempt(t *testing.T, s *Session, to string) *Attempt {
	t.Helper()
	a, err := s.Prepare(AttemptOptions{Protocol: to, Model: "selected", PublicModel: "public"})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestTranslationSessionMatrix(t *testing.T) {
	for from, raw := range sessionRequests {
		for to := range sessionRequests {
			t.Run(from+"/"+to, func(t *testing.T) {
				s := newTestSession(t, raw, from)
				a := prepareTestAttempt(t, s, to)
				m, _ := Decode(a.RequestBody())
				if m["model"] != "selected" {
					t.Fatal(m)
				}
				r, err := a.TranslateResponse(strings.NewReader(sessionReplies[to]))
				if err != nil || !r.Usage.Known || r.Usage.Input != 0 || r.Observation.Status != "completed" {
					t.Fatalf("%+v %v", r, err)
				}
				if !strings.Contains(string(r.Body), "hello") {
					t.Fatal(string(r.Body))
				}
				if from == to && string(r.Body) != sessionReplies[to] {
					t.Fatal("native response modified")
				}
				var w bytes.Buffer
				r, err = a.TranslateStream(t.Context(), &w, strings.NewReader(sessionStreams[to]), StreamOptions{})
				if err != nil || !r.Usage.Known || !strings.Contains(w.String(), "hello") {
					t.Fatalf("%+v %v %s", r, err, w.String())
				}
			})
		}
	}
}

func TestTranslationNativeFieldsAndCrossRejection(t *testing.T) {
	for from, raw := range sessionRequests {
		t.Run(from, func(t *testing.T) {
			m, _ := Decode([]byte(raw))
			m["native_extension"] = Object{"opaque": "keep"}
			if from == "anthropic" {
				m["thinking"] = Object{"type": "enabled", "budget_tokens": 1024}
				m["messages"] = []any{Object{"role": "assistant", "content": []any{Object{"type": "thinking", "thinking": "secret", "signature": "opaque-signature"}}}}
			}
			if from == "chat-completions" {
				m["stream_options"] = Object{"include_usage": false}
				m["n"] = 2
			}
			rawBytes, _ := json.Marshal(m)
			s := newTestSession(t, string(rawBytes), from)
			a := prepareTestAttempt(t, s, from)
			got, _ := Decode(a.RequestBody())
			delete(got, "model")
			delete(m, "model")
			if from == "responses" {
				m["store"] = false
			}
			want, _ := json.Marshal(m)
			actual, _ := json.Marshal(got)
			if string(actual) != string(want) {
				t.Fatalf("native loss: %s != %s", actual, want)
			}
			for target := range sessionRequests {
				if target != from {
					if _, err := s.Prepare(AttemptOptions{Protocol: target}); err == nil {
						t.Fatal("unknown native fields silently dropped")
					}
				}
			}
		})
	}
	for _, raw := range []string{`{"model":"m"}`, `{"messages":[]}`, `{"messages":[{"role":"invalid"}]}`} {
		if _, err := NewSession([]byte(raw), "chat-completions"); err == nil {
			t.Fatal("invalid ingress accepted", raw)
		}
	}
}

func TestTranslationSessionIsolation(t *testing.T) {
	raw := []byte(`{"model":"public","input":"hello","tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"lookup","parameters":{}}]}]}`)
	s, err := NewSession(raw, "responses")
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = 'x'
	view := s.PolicyFields()
	view["model"][0] = 'x'
	view["tools"][0] = 'x'
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target := []string{"responses", "chat-completions", "anthropic"}[i%3]
			a, err := s.Prepare(AttemptOptions{Protocol: target, Model: fmt.Sprint(i), PublicModel: "public"})
			if err != nil {
				t.Error(err)
				return
			}
			wire := a.RequestBody()
			m, _ := Decode(wire)
			wire[0] = 'x'
			if m["model"] != fmt.Sprint(i) || a.RequestBody()[0] != '{' {
				t.Error("request baseline leaked")
			}
			if target == "chat-completions" {
				reply := `{"id":"r","choices":[{"finish_reason":"tool_calls","message":{"tool_calls":[{"id":"c","type":"function","function":{"name":"functions__lookup","arguments":"{}"}}]}}]}`
				r, err := a.TranslateResponse(strings.NewReader(reply))
				if err != nil || !strings.Contains(string(r.Body), `"namespace":"functions"`) {
					t.Errorf("mapping leaked: %s %v", r.Body, err)
				}
			}
		}(i)
	}
	wg.Wait()
	// Same alias in a different request is a plain function, not the old namespace.
	other := newTestSession(t, `{"model":"public","input":"hello","tools":[{"type":"function","name":"functions__lookup","parameters":{}}]}`, "responses")
	a := prepareTestAttempt(t, other, "chat-completions")
	reply := `{"id":"r","choices":[{"finish_reason":"tool_calls","message":{"tool_calls":[{"id":"c","type":"function","function":{"name":"functions__lookup","arguments":"{}"}}]}}]}`
	r, err := a.TranslateResponse(strings.NewReader(reply))
	if err != nil || strings.Contains(string(r.Body), `"namespace"`) {
		t.Fatalf("cross-request mapping leak: %s %v", r.Body, err)
	}
	// A failed candidate must not contaminate either the session or a later native one.
	failed := newTestSession(t, `{"model":"public","input":"hello","reasoning":{"effort":"high"},"tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"lookup","parameters":{}}]}]}`, "responses")
	if _, err := failed.Prepare(AttemptOptions{Protocol: "anthropic"}); err == nil {
		t.Fatal("unsupported reasoning options accepted")
	}
	if got := string(prepareTestAttempt(t, failed, "responses").RequestBody()); !strings.Contains(got, `"type":"namespace"`) {
		t.Fatal(got)
	}
}

func TestTranslationDiagnosticsAndUsage(t *testing.T) {
	s := newTestSession(t, `{"input":"PRIVATE_PROMPT","reasoning":{"summary":"auto"},"include":["reasoning.encrypted_content"],"tools":[{"type":"custom","name":"PRIVATE_TOOL","format":{"type":"grammar","syntax":"lark","definition":"PRIVATE_GRAMMAR"}}]}`, "responses")
	a := prepareTestAttempt(t, s, "chat-completions")
	d := a.Diagnostics()
	if d.Compatibility != Degraded || len(d.Degradations) != 3 {
		t.Fatal(d)
	}
	d.Degradations[0] = "PRIVATE"
	if a.Diagnostics().Degradations[0] == "PRIVATE" {
		t.Fatal("diagnostic state escaped")
	}
	for _, tc := range []struct {
		raw   string
		known bool
	}{
		{`{"id":"r","choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`, false},
		{sessionReplies["chat-completions"], true},
	} {
		r, err := a.TranslateResponse(strings.NewReader(tc.raw))
		if err != nil || r.Usage.Known != tc.known {
			t.Fatal(r, err)
		}
	}
	for _, raw := range []string{`PRIVATE_NOT_JSON`, `{"error":{"message":"PRIVATE_UPSTREAM"}}`, `{"id":"r","choices":[{"finish_reason":"PRIVATE_FINISH","message":{}}]}`} {
		r, err := a.TranslateResponse(strings.NewReader(raw))
		if err == nil || len(r.Body) > 0 {
			t.Fatal("bad response accepted")
		}
		b, _ := json.Marshal(ErrorDiagnostics(err))
		if strings.Contains(string(b)+err.Error(), "PRIVATE") {
			t.Fatal("diagnostic leaked")
		}
		if ErrorDiagnostics(err).Stage != StageResponse {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{`{"input":"x","reasoning":{"PRIVATE_FIELD":"PRIVATE_VALUE"}}`, `{"input":[{"type":"reasoning","encrypted_content":"PRIVATE_CIPHERTEXT"}]}`} {
		s := newTestSession(t, raw, "responses")
		_, err := s.Prepare(AttemptOptions{Protocol: "chat-completions"})
		if err == nil || strings.Contains(err.Error(), "PRIVATE") || ErrorDiagnostics(err).Compatibility != Unsupported {
			t.Fatal(err)
		}
	}
	a = prepareTestAttempt(t, newTestSession(t, sessionRequests["responses"], "responses"), "responses")
	for _, raw := range []string{`{"status":"in_progress"}`, `{"status":"failed"}`, `{"status":"queued"}`} {
		if _, err := a.TranslateResponse(strings.NewReader(raw)); err == nil {
			t.Fatal("HTTP 200 treated as complete", raw)
		}
	}
}

type stagedTranslationReader struct {
	chunks []string
	output *bytes.Buffer
	reads  int
	t      *testing.T
}

func (r *stagedTranslationReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	if r.reads == 1 && !strings.Contains(r.output.String(), "hello") {
		r.t.Error("text buffered until completion")
	}
	part := r.chunks[0]
	n := copy(p, part)
	r.chunks[0] = part[n:]
	if r.chunks[0] == "" {
		r.chunks = r.chunks[1:]
		r.reads++
	}
	return n, nil
}

type failTranslationWriter struct{}

func (failTranslationWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestTranslationStreamIncrementalCancellationAndLimits(t *testing.T) {
	a := prepareTestAttempt(t, newTestSession(t, sessionRequests["responses"], "responses"), "chat-completions")
	var out bytes.Buffer
	reader := &stagedTranslationReader{output: &out, t: t, chunks: []string{sse(`{"choices":[{"index":0,"delta":{"content":"hello"}}]}`), sse(`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`, `[DONE]`)}}
	if _, err := a.TranslateStream(t.Context(), &out, reader, StreamOptions{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	out.Reset()
	r, err := a.TranslateStream(ctx, &out, strings.NewReader(sessionStreams["chat-completions"]), StreamOptions{})
	if !errors.Is(err, context.Canceled) || r.Diagnostics.Category != "cancelled" || out.Len() != 0 {
		t.Fatal(r, err, out.String())
	}
	if _, err := a.TranslateStream(t.Context(), failTranslationWriter{}, strings.NewReader(sessionStreams["chat-completions"]), StreamOptions{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"truncated":      sse(`{"choices":[{"index":0,"delta":{"content":"hello"}}]}`),
		"missing_finish": sse(`{"choices":[{"index":0,"delta":{"content":"hello"}}]}`, `[DONE]`),
		"invalid_args":   sse(`{"choices":[{"index":0,"finish_reason":"tool_calls","delta":{"tool_calls":[{"index":0,"id":"c","function":{"name":"f","arguments":"{\"x\":"}}]}}]}`, `[DONE]`),
		"event_limit":    "data: " + strings.Repeat("x", 2<<20) + "\n\n",
		"tool_limit": func() string {
			var b strings.Builder
			for i := 0; i < 257; i++ {
				b.WriteString(sse(fmt.Sprintf(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":%d,"id":"c%d","function":{"name":"f","arguments":"{}"}}]}}]}`, i, i)))
			}
			return b.String()
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			out.Reset()
			_, err := a.TranslateStream(t.Context(), &out, strings.NewReader(data), StreamOptions{})
			if err == nil || strings.Contains(out.String(), `"type":"response.completed"`) {
				t.Fatalf("%v %s", err, out.String())
			}
		})
	}
	// Limit the accumulated native wire size even when no converted text accumulates.
	a = prepareTestAttempt(t, newTestSession(t, sessionRequests["responses"], "responses"), "responses")
	event := sse(`{"type":"extension","padding":"` + strings.Repeat("x", 1<<20) + `"}`)
	_, err = a.TranslateStream(t.Context(), io.Discard, io.MultiReader(&repeatTranslationReader{s: event, n: 65}, strings.NewReader(sessionStreams["responses"])), StreamOptions{})
	if err == nil {
		t.Fatal("native wire limit not enforced")
	}
}

type repeatTranslationReader struct {
	s      string
	n      int
	offset int
}

func (r *repeatTranslationReader) Read(p []byte) (int, error) {
	if r.n == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.offset:])
	r.offset += n
	if r.offset == len(r.s) {
		r.n--
		r.offset = 0
	}
	return n, nil
}

func TestTranslationIngressPolicyCannotBeBypassedByNativeFields(t *testing.T) {
	for _, tc := range []struct {
		from, raw, category string
		tools               bool
	}{
		{"responses", `{"model":"m","input":[{"type":"additional_tools","tools":[{"type":"function","name":"f","parameters":{}}]}]}`, "tools_disabled", false},
		{"responses", `{"model":"m","input":[{"type":"additional_tools","tools":[{"type":"web_search"}]}]}`, "hosted_tool", true},
		{"responses", `{"model":"m","input":[0,{"type":"additional_tools","tools":[{"type":"web_search"}]}]}`, "hosted_tool", true},
		{"chat-completions", `{"model":"m","messages":[{"role":"user","content":"x"}],"native_field":true,"tools":[{"type":"web_search"}]}`, "hosted_tool", true},
		{"anthropic", `{"model":"m","messages":[{"role":"user","content":"x"}],"tools":[{"type":"web_search_20250305","name":"web_search"}]}`, "hosted_tool", true},
		{"chat-completions", `{"model":"m","messages":[{"role":"user","content":"x"}],"store":true}`, "stateful_request", true},
		{"responses", `{"model":"m","input":"x","previous_response_id":"PRIVATE_ID"}`, "stateful_request", true},
	} {
		t.Run(tc.from+tc.category, func(t *testing.T) {
			s := newTestSession(t, tc.raw, tc.from)
			err := s.CheckCapabilities(Capabilities{Tools: tc.tools, Streaming: true})
			if err == nil || ErrorDiagnostics(err).Category != tc.category {
				t.Fatal(err, ErrorDiagnostics(err))
			}
			if RequestErrorID(err) == "" {
				t.Fatal("lost existing API code")
			}
		})
	}
}

func TestTranslationAccumulatedAndResponseLimits(t *testing.T) {
	s := newTestSession(t, sessionRequests["responses"], "responses")
	a := prepareTestAttempt(t, s, "chat-completions")
	event := sse(`{"choices":[{"index":0,"delta":{"content":"` + strings.Repeat("x", 1<<20) + `"}}]}`)
	_, err := a.TranslateStream(t.Context(), io.Discard, &repeatTranslationReader{s: event, n: 17}, StreamOptions{})
	if err == nil {
		t.Fatal("unbounded converted content")
	}
	// A successful HTTP body with an unbounded payload must never reach delivery.
	r, err := a.TranslateResponse(&repeatTranslationReader{s: strings.Repeat(" ", 1<<20), n: 17})
	if err == nil || len(r.Body) > 0 || ErrorDiagnostics(err).Category != "incomplete_body" {
		t.Fatal(r.Diagnostics, err)
	}
}

// The owner of the network/body must close a blocked read on cancellation.
// This verifies the converter exits without an internal reader goroutine.
type translationBlockedReader struct {
	io.Reader
	reads   int
	blocked chan struct{}
}

func (r *translationBlockedReader) Read(p []byte) (int, error) {
	r.reads++
	if r.reads == 2 {
		close(r.blocked)
	}
	return r.Reader.Read(p)
}
func TestTranslationBlockedReadCleanup(t *testing.T) {
	a := prepareTestAttempt(t, newTestSession(t, sessionRequests["responses"], "responses"), "chat-completions")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	input := &translationBlockedReader{Reader: reader, blocked: make(chan struct{})}
	finished := make(chan error, 1)
	go func() { _, err := a.TranslateStream(ctx, io.Discard, input, StreamOptions{}); finished <- err }()
	if _, err := io.WriteString(writer, sse(`{"choices":[{"index":0,"delta":{"content":"partial"}}]}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-input.blocked:
	case <-time.After(time.Second):
		t.Fatal("reader did not block")
	}
	cancel()
	reader.CloseWithError(ctx.Err())
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("converter leaked after body close")
	}
}

func TestTranslationMissingMessagesUsageAndNativeDefaults(t *testing.T) {
	s := newTestSession(t, `{"model":"m","messages":[{"role":"user","content":"x"}]}`, "anthropic")
	m, _ := Decode(prepareTestAttempt(t, s, "anthropic").RequestBody())
	if m["max_tokens"] != float64(4096) {
		t.Fatal("lost existing default", m)
	}
	a := prepareTestAttempt(t, s, "chat-completions")
	r, err := a.TranslateResponse(strings.NewReader(`{"id":"r","choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`))
	if err == nil || ErrorDiagnostics(err).Category != "missing_usage" || r.Usage.Known {
		t.Fatal(err, r)
	}
}

type translationCheckpointReader struct {
	chunks  []string
	markers []string
	index   int
	output  *bytes.Buffer
	t       *testing.T
}

func (r *translationCheckpointReader) Read(p []byte) (int, error) {
	if r.index >= len(r.chunks) {
		return 0, io.EOF
	}
	if r.index > 0 && !strings.Contains(r.output.String(), r.markers[r.index-1]) {
		r.t.Errorf("block %d was buffered past its source boundary", r.index-1)
	}
	n := copy(p, r.chunks[r.index])
	r.chunks[r.index] = r.chunks[r.index][n:]
	if r.chunks[r.index] == "" {
		r.index++
	}
	return n, nil
}
func TestTranslationSequentialAnthropicBlocksIncrementally(t *testing.T) {
	a := prepareTestAttempt(t, newTestSession(t, sessionRequests["anthropic"], "anthropic"), "responses")
	var out bytes.Buffer
	input := &translationCheckpointReader{output: &out, t: t, markers: []string{"FIRST_TEXT", "SECOND_TEXT"}, chunks: []string{
		sse(`{"type":"response.created","response":{"id":"r"}}`, `{"type":"response.output_item.added","output_index":0,"item":{"type":"message"}}`, `{"type":"response.output_text.delta","output_index":0,"delta":"FIRST_TEXT"}`, `{"type":"response.output_item.done","output_index":0}`),
		sse(`{"type":"response.output_item.added","output_index":1,"item":{"type":"message"}}`, `{"type":"response.output_text.delta","output_index":1,"delta":"SECOND_TEXT"}`, `{"type":"response.output_item.done","output_index":1}`),
		sse(`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":1,"output_tokens":2}}}`),
	}}
	if _, err := a.TranslateStream(t.Context(), &out, input, StreamOptions{}); err != nil {
		t.Fatal(err, out.String())
	}
	for _, event := range []string{"content_block_start", "content_block_stop"} {
		// Each block is opened and closed once, including before the message terminal.
		if strings.Count(out.String(), "event: "+event+"\n") != 2 {
			t.Fatal(out.String())
		}
	}
}
