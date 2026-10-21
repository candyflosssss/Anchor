package core

import (
	"bufio"
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

func TestTranslationGatewayToolLoopAndCandidateSwitch(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			f := setup(t)
			var failures, calls atomic.Int32
			bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { failures.Add(1); w.WriteHeader(503) }))
			defer bad.Close()
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				var m protocol.Object
				if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
					t.Error(err)
				}
				if r.URL.Path != "/chat/completions" || m["model"] != "chat-model" {
					t.Errorf("bad attempt %s %+v", r.URL.Path, m)
				}
				tools := m["tools"].([]any)
				names := make([]string, len(tools))
				for i, v := range tools {
					names[i] = v.(map[string]any)["function"].(map[string]any)["name"].(string)
				}
				if names[0] != "functions__lookup" || names[1] != "functions__patch" {
					t.Error(names)
				}
				if n == 2 {
					messages := m["messages"].([]any)
					history, _ := json.Marshal(messages)
					if !strings.Contains(string(history), `"tool_call_id":"c0"`) || !strings.Contains(string(history), `"tool_call_id":"c1"`) || !strings.Contains(string(history), `"name":"functions__lookup"`) || !strings.Contains(string(history), `"name":"functions__patch"`) || !strings.Contains(string(history), `PRIVATE_RESULT`) {
						t.Errorf("tool results lost: %s", history)
					}
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					send := func(v any) { b, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", b); w.(http.Flusher).Flush() }
					if n == 1 {
						for i, name := range names {
							args := `{"x":"雪😀"}`
							if i == 1 {
								args = `{"input":"line\n雪😀\"\\"}`
							}
							// Split inside a JSON escape. Individual events stay valid JSON.
							split := 3
							if i == 1 {
								split = strings.Index(args, `\n`) + 1
							}
							send(protocol.Object{"id": "r", "choices": []any{protocol.Object{"index": 0, "delta": protocol.Object{"tool_calls": []any{protocol.Object{"index": i, "id": fmt.Sprint("c", i), "type": "function", "function": protocol.Object{"name": name, "arguments": args[:split]}}}}}}})
							send(protocol.Object{"choices": []any{protocol.Object{"index": 0, "delta": protocol.Object{"tool_calls": []any{protocol.Object{"index": i, "function": protocol.Object{"arguments": args[split:]}}}}}}})
						}
						send(protocol.Object{"choices": []any{protocol.Object{"index": 0, "delta": protocol.Object{}, "finish_reason": "tool_calls"}}})
					} else {
						send(protocol.Object{"id": "final", "choices": []any{protocol.Object{"index": 0, "delta": protocol.Object{"content": "FINAL_REPLY"}, "finish_reason": "stop"}}})
					}
					send(protocol.Object{"choices": []any{}, "usage": protocol.Object{"prompt_tokens": 3, "completion_tokens": 2}})
					io.WriteString(w, "data: [DONE]\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				msg := protocol.Object{"content": "FINAL_REPLY"}
				finish := "stop"
				if n == 1 {
					finish = "tool_calls"
					msg = protocol.Object{"content": nil, "tool_calls": []any{
						protocol.Object{"id": "c0", "type": "function", "function": protocol.Object{"name": names[0], "arguments": `{"x":"雪😀"}`}},
						protocol.Object{"id": "c1", "type": "function", "function": protocol.Object{"name": names[1], "arguments": `{"input":"line\n雪😀\"\\"}`}},
					}}
				}
				json.NewEncoder(w).Encode(protocol.Object{"id": "r", "choices": []any{protocol.Object{"message": msg, "finish_reason": finish}}, "usage": protocol.Object{"prompt_tokens": 3, "completion_tokens": 2}})
			}))
			defer up.Close()
			f.add(t, "first", bad.URL, "synthetic")
			f.add(t, "second", up.URL, "synthetic")
			if err := f.store.PutKeyModelProtocol("second", 1, "default", "chat-model", "chat-completions"); err != nil {
				t.Fatal(err)
			}
			f.model(t, state.Candidate{ProviderID: "first", Model: "native-model"}, state.Candidate{ProviderID: "second", Model: "chat-model"})
			tools := `[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}},{"type":"custom","name":"patch"}]}]`
			ingress := fmt.Sprintf(`{"model":"public","stream":%t,"input":"PRIVATE_PROMPT","tools":%s}`, stream, tools)
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, ingress)
			body := text(t, resp)
			if resp.StatusCode != 200 {
				t.Fatal(resp.StatusCode, body)
			}
			var result protocol.Object
			if !stream {
				if err := json.Unmarshal([]byte(body), &result); err != nil {
					t.Fatal(err)
				}
			} else {
				scanner := bufio.NewScanner(strings.NewReader(body))
				for scanner.Scan() {
					line := scanner.Text()
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					var event protocol.Object
					json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event)
					if event["type"] == "response.completed" {
						result = event["response"].(map[string]any)
					}
				}
			}
			if result == nil {
				t.Fatal("missing completion", body)
			}
			output := result["output"].([]any)
			if len(output) != 2 || output[0].(map[string]any)["namespace"] != "functions" || output[1].(map[string]any)["type"] != "custom_tool_call" || output[1].(map[string]any)["input"] != "line\n雪😀\"\\" {
				t.Fatalf("identity/input lost: %+v", output)
			}
			history := append([]any{protocol.Object{"role": "user", "content": "PRIVATE_PROMPT"}}, output...)
			history = append(history, protocol.Object{"type": "function_call_output", "call_id": "c0", "output": "PRIVATE_RESULT"}, protocol.Object{"type": "custom_tool_call_output", "call_id": "c1", "output": "PRIVATE_RESULT"})
			follow, _ := json.Marshal(protocol.Object{"model": "public", "stream": stream, "input": history, "tools": json.RawMessage(tools)})
			resp = request(t, "POST", f.gateway.URL+"/v1/responses", f.token, string(follow))
			body = text(t, resp)
			if resp.StatusCode != 200 || !strings.Contains(body, "FINAL_REPLY") || calls.Load() != 2 || failures.Load() < 1 {
				t.Fatal(resp.StatusCode, body, calls.Load(), failures.Load())
			}
			f.core.logs.Flush(t.Context())
			detail, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
			if err != nil {
				t.Fatal(err)
			}
			if detail.Evidence["translation_client"] != "responses" || detail.Evidence["conversion_target"] != "chat-completions" || detail.Evidence["translation_dispatched"] != "true" || detail.Evidence["translation_delivered"] != "true" {
				t.Fatalf("missing diagnostics %+v", detail)
			}
			stored, _ := json.Marshal(detail)
			if strings.Contains(string(stored)+f.logs.String(), "PRIVATE_") {
				t.Fatal("private content persisted")
			}
		})
	}
}

func TestTranslationGatewayNativeAndFailureDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, body, stage, category string
		status                      int
		stream                      bool
	}{
		{"native", `{"id":"r","status":"completed","output":[{"type":"reasoning","encrypted_content":"opaque"}],"native_extension":true}`, "response", "", 200, false},
		{"invalid_json", `PRIVATE_UPSTREAM_NOT_JSON`, "response", "invalid_json", 502, false},
		{"nonterminal", `{"id":"r","status":"in_progress"}`, "response", "upstream_error", 502, false},
		{"interrupted", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"PRIVATE_TEXT\"}\n\n", "stream", "missing_terminal", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
				}
				io.WriteString(w, tc.body)
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "synthetic")
			f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, fmt.Sprintf(`{"model":"public","input":"PRIVATE_PROMPT","stream":%t}`, tc.stream))
			body := text(t, resp)
			if resp.StatusCode != tc.status || calls.Load() != 1 {
				t.Fatal(resp.StatusCode, body, calls.Load())
			}
			if tc.name == "native" && body != tc.body {
				t.Fatal("native response was re-encoded", body)
			}
			f.core.logs.Flush(t.Context())
			detail, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
			if err != nil {
				t.Fatal(err)
			}
			d := detail.Evidence
			if d["translation_stage"] != tc.stage || d["translation_category"] != tc.category || d["translation_dispatched"] != "true" {
				t.Fatal(d)
			}
			if tc.category != "" && !tc.stream && d["translation_delivered"] != "false" {
				t.Fatal(d)
			}
			b, _ := json.Marshal(detail)
			if strings.Contains(string(b)+f.logs.String(), "PRIVATE_") {
				t.Fatal("failure diagnostic leaked")
			}
		})
	}
}
