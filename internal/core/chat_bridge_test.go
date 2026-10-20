package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestGatewayCodexChatBridge(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			f := setup(t)
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var m protocol.Object
				if json.NewDecoder(r.Body).Decode(&m) != nil {
					t.Error("invalid request")
				}
				if r.URL.Path != "/v1/chat/completions" || m["model"] != "deepseek-test" || m["reasoning_effort"] != "high" || m["reasoning"] != nil || m["include"] != nil {
					t.Errorf("request=%+v path=%s", m, r.URL.Path)
				}
				tools := m["tools"].([]any)
				function := tools[0].(map[string]any)["function"].(map[string]any)
				name := function["name"].(string)
				if !strings.Contains(function["description"].(string), "start:") {
					t.Error("custom grammar was lost")
				}
				if !stream {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id": "r", "choices": []any{map[string]any{"finish_reason": "tool_calls", "message": map[string]any{
							"reasoning_content": "PRIVATE_REASONING", "content": "visible reply", "tool_calls": []any{map[string]any{"id": "call", "type": "function", "function": map[string]any{"name": name, "arguments": `{"input":"PRIVATE_ARGUMENT"}`}}},
						}}}, "usage": map[string]any{"prompt_tokens": 5, "completion_tokens": 4, "completion_tokens_details": map[string]any{"reasoning_tokens": 2}},
					})
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: "+`{"id":"r","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"PRIVATE_REASONING"}}]}`+"\n\n")
				io.WriteString(w, "data: "+`{"choices":[{"index":0,"delta":{"content":"visible reply"}}]}`+"\n\n")
				chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "finish_reason": "tool_calls", "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call", "type": "function", "function": map[string]any{"name": name, "arguments": `{"input":"PRIVATE_ARGUMENT"}`}}}}}}})
				fmt.Fprintf(w, "data: %s\n\n", chunk)
				io.WriteString(w, "data: "+`{"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":4,"completion_tokens_details":{"reasoning_tokens":2}}}`+"\n\ndata: [DONE]\n\n")
			}))
			defer up.Close()
			f.add(t, "p", up.URL+"/v1", "synthetic-secret")
			if err := f.store.PutKeyModelProtocol("p", 1, "default", "deepseek-test", "chat-completions"); err != nil {
				t.Fatal(err)
			}
			f.model(t, state.Candidate{ProviderID: "p", Model: "deepseek-test"})
			body := fmt.Sprintf(`{"model":"public","stream":%t,"store":false,"input":"PRIVATE_PROMPT","reasoning":{"effort":"high","summary":"auto"},"text":{"format":{"type":"text"}},"include":["reasoning.encrypted_content"],"tools":[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"patch","format":{"type":"grammar","syntax":"lark","definition":"start: /.+/"}}]}]}`, stream)
			r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, body)
			raw := text(t, r)
			if r.StatusCode != 200 || calls.Load() != 1 || !strings.Contains(raw, `"type":"custom_tool_call"`) || !strings.Contains(raw, `"name":"patch"`) || !strings.Contains(raw, `"namespace":"functions"`) || !strings.Contains(raw, `"input":"PRIVATE_ARGUMENT"`) || !strings.Contains(raw, `"type":"reasoning"`) || strings.Contains(raw, "upstream_stream_error") {
				t.Fatal(r.StatusCode, calls.Load(), raw)
			}
			if stream && !strings.Contains(raw, `"type":"response.completed"`) {
				t.Fatal("missing completion", raw)
			}
			if err := f.core.logs.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			detail, err := logClient(t, f).LogDetail(t.Context(), r.Header.Get("X-Anchor-Request-ID"))
			if err != nil || detail.Status != "succeeded" || detail.AttemptCount != 1 || detail.Attempts[0].Protocol != "chat-completions" || detail.Metrics.ReasoningTokens == nil || *detail.Metrics.ReasoningTokens != 2 {
				t.Fatalf("detail=%+v err=%v", detail, err)
			}
			stored, _ := json.Marshal(detail)
			for _, secret := range []string{"PRIVATE_REASONING", "PRIVATE_PROMPT", "PRIVATE_ARGUMENT", "synthetic-secret"} {
				if strings.Contains(string(stored), secret) || strings.Contains(f.logs.String(), secret) {
					t.Fatal("inference content leaked into logs")
				}
			}
		})
	}
}

func TestGatewayConversionFailureHasSafeRouteDiagnostics(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer up.Close()
	f.add(t, "p", up.URL+"/v1", "synthetic-secret")
	if err := f.store.PutKeyModelProtocol("p", 1, "default", "up", "chat-completions"); err != nil {
		t.Fatal(err)
	}
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	for _, tc := range []struct{ body, reason string }{
		{`{"model":"public","input":"PRIVATE_PROMPT","private-field-synthetic-secret":"PRIVATE_VALUE"}`, "request_fields"},
		{`{"model":"public","input":"PRIVATE_PROMPT","reasoning":{"private-field-synthetic-secret":"PRIVATE_VALUE"}}`, "reasoning_options"},
		{`{"model":"public","input":[{"type":"reasoning","summary":[],"encrypted_content":"PRIVATE_CIPHERTEXT"}]}`, "encrypted_reasoning_history"},
	} {
		r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, tc.body)
		raw := text(t, r)
		if r.StatusCode != 400 || !strings.Contains(raw, tc.reason) || calls.Load() != 0 {
			t.Fatal(r.StatusCode, raw, calls.Load())
		}
		if err := f.core.logs.Flush(t.Context()); err != nil {
			t.Fatal(err)
		}
		detail, err := logClient(t, f).LogDetail(t.Context(), r.Header.Get("X-Anchor-Request-ID"))
		if err != nil || detail.Code != "unsupported_conversion" || detail.Stage != "request_convert" || detail.Provider != "p" || detail.Key != "default" || detail.UpstreamModel != "up" || detail.HTTP != nil || detail.AttemptCount != 0 || detail.Evidence["conversion_target"] != "chat-completions" || detail.Evidence["conversion_reason"] != tc.reason || detail.Evidence["sent_service_tier"] != "not_sent" {
			t.Fatalf("%+v %v", detail, err)
		}
		stored, _ := json.Marshal(detail)
		if strings.Contains(raw+string(stored)+f.logs.String(), "PRIVATE_") || strings.Contains(raw+string(stored)+f.logs.String(), "synthetic-secret") || strings.Contains(raw+string(stored), "private-field") {
			t.Fatal("unsafe conversion error")
		}
	}
}
