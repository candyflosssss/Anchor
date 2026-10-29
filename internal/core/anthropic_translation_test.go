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

func TestAnthropicGatewayMappedToolLoop(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			f := setup(t)
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				var m protocol.Object
				if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
					t.Error(err)
				}
				if r.URL.Path != "/messages" || m["model"] != "claude-up" || r.Header.Get("Anthropic-Version") == "" {
					t.Error(r.URL.Path, m)
				}
				if n == 2 {
					history, _ := json.Marshal(m["messages"])
					if !strings.Contains(string(history), `"tool_use_id":"c"`) || !strings.Contains(string(history), `"name":"functions__patch"`) || !strings.Contains(string(history), "PRIVATE_RESULT") {
						t.Error(string(history))
					}
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					send := func(v any) { b, _ := json.Marshal(v); fmt.Fprintf(w, "data: %s\n\n", b); w.(http.Flusher).Flush() }
					send(protocol.Object{"type": "message_start", "message": protocol.Object{"id": "r", "usage": protocol.Object{"input_tokens": 2, "output_tokens": 0, "cache_read_input_tokens": 3, "cache_creation_input_tokens": 1}}})
					if n == 1 {
						send(protocol.Object{"type": "content_block_start", "index": 0, "content_block": protocol.Object{"type": "tool_use", "id": "c", "name": "functions__patch", "input": protocol.Object{"input": "line\n雪😀"}}})
					} else {
						send(protocol.Object{"type": "content_block_start", "index": 0, "content_block": protocol.Object{"type": "thinking", "thinking": "PRIVATE_THINKING", "signature": "PRIVATE_SIGNATURE"}})
					}
					send(protocol.Object{"type": "content_block_stop", "index": 0})
					if n == 2 {
						send(protocol.Object{"type": "content_block_start", "index": 1, "content_block": protocol.Object{"type": "text", "text": "FINAL_REPLY"}})
						send(protocol.Object{"type": "content_block_stop", "index": 1})
					}
					finish := "end_turn"
					if n == 1 {
						finish = "tool_use"
					}
					send(protocol.Object{"type": "message_delta", "delta": protocol.Object{"stop_reason": finish}, "usage": protocol.Object{"input_tokens": 2, "output_tokens": 4}})
					send(protocol.Object{"type": "message_stop"})
					return
				}
				w.Header().Set("Content-Type", "application/json")
				content := []any{protocol.Object{"type": "thinking", "thinking": "PRIVATE_THINKING", "signature": "PRIVATE_SIGNATURE"}, protocol.Object{"type": "text", "text": "FINAL_REPLY"}}
				finish := "end_turn"
				if n == 1 {
					finish = "tool_use"
					content = []any{protocol.Object{"type": "tool_use", "id": "c", "name": "functions__patch", "input": protocol.Object{"input": "line\n雪😀"}}}
				}
				json.NewEncoder(w).Encode(protocol.Object{"id": "r", "content": content, "stop_reason": finish, "usage": protocol.Object{"input_tokens": 2, "output_tokens": 4, "cache_read_input_tokens": 3, "cache_creation_input_tokens": 1}})
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "synthetic")
			if err := f.store.PutKeyModelProtocol("p", 1, "default", "claude-up", "anthropic"); err != nil {
				t.Fatal(err)
			}
			f.model(t, state.Candidate{ProviderID: "p", Model: "claude-up"})
			tools := `[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"patch"}]}]`
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, fmt.Sprintf(`{"model":"public","input":"PRIVATE_PROMPT","stream":%t,"tools":%s}`, stream, tools))
			wire := text(t, resp)
			if resp.StatusCode != 200 {
				t.Fatal(resp.StatusCode, wire)
			}
			var result protocol.Object
			if stream {
				scanner := bufio.NewScanner(strings.NewReader(wire))
				for scanner.Scan() {
					if strings.HasPrefix(scanner.Text(), "data: ") {
						var event protocol.Object
						if json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data: ")), &event) == nil && event["type"] == "response.completed" {
							result, _ = event["response"].(map[string]any)
						}
					}
				}
				if err := scanner.Err(); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := json.Unmarshal([]byte(wire), &result); err != nil {
					t.Fatal(err)
				}
			}
			items, _ := result["output"].([]any)
			if len(items) != 1 {
				t.Fatal(wire)
			}
			call, _ := items[0].(map[string]any)
			if call["type"] != "custom_tool_call" || call["namespace"] != "functions" || call["input"] != "line\n雪😀" {
				t.Fatal(call)
			}
			history := []any{protocol.Object{"role": "user", "content": "PRIVATE_PROMPT"}, call, protocol.Object{"type": "custom_tool_call_output", "call_id": "c", "output": "PRIVATE_RESULT"}}
			input, _ := json.Marshal(history)
			resp = request(t, "POST", f.gateway.URL+"/v1/responses", f.token, fmt.Sprintf(`{"model":"public","input":%s,"stream":%t,"tools":%s}`, input, stream, tools))
			wire = text(t, resp)
			if resp.StatusCode != 200 || calls.Load() != 2 || !strings.Contains(wire, "FINAL_REPLY") || !strings.Contains(wire, `"type":"reasoning"`) || strings.Contains(wire, "PRIVATE_SIGNATURE") {
				t.Fatal(resp.StatusCode, calls.Load(), wire)
			}
			f.core.logs.Flush(t.Context())
			detail, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
			if err != nil {
				t.Fatal(err)
			}
			if detail.Evidence["translation_compatibility"] != "degraded" || !strings.Contains(detail.Evidence["translation_degradations"], "anthropic_thinking_plaintext") {
				t.Fatal(detail.Evidence)
			}
			stored, _ := json.Marshal(detail)
			if strings.Contains(string(stored)+f.logs.String(), "PRIVATE_") {
				t.Fatal("private content persisted")
			}
		})
	}
}

func TestAnthropicGatewayCacheHintsAndToolErrorDiagnostics(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/chat/completions" || strings.Contains(string(raw), "cache_control") || !strings.Contains(string(raw), `"parallel_tool_calls":false`) || !strings.Contains(string(raw), `[Tool execution failed]\nPRIVATE_RESULT`) || r.Header.Get("Anthropic-Beta") != "" {
			t.Error(r.URL.Path, string(raw))
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	if err := f.store.PutKeyModelProtocol("p", 1, "default", "chat-up", "chat-completions"); err != nil {
		t.Fatal(err)
	}
	f.model(t, state.Candidate{ProviderID: "p", Model: "chat-up"})
	raw := `{"model":"public","tools":[{"type":"custom","name":"lookup","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}],"tool_choice":{"type":"auto","disable_parallel_tool_use":true},"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"c","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"c","is_error":true,"content":"PRIVATE_RESULT"}]}]}`
	resp := request(t, "POST", f.gateway.URL+"/v1/messages", f.token, raw)
	wire := text(t, resp)
	if resp.StatusCode != 200 || !strings.Contains(wire, `"stop_reason":"end_turn"`) {
		t.Fatal(resp.StatusCode, wire)
	}
	f.core.logs.Flush(t.Context())
	detail, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
	if err != nil {
		t.Fatal(err)
	}
	if detail.Evidence["translation_compatibility"] != "degraded" || detail.Evidence["translation_degradations"] != "prompt_cache_control,tool_result_error_text" {
		t.Fatal(detail.Evidence)
	}
	stored, _ := json.Marshal(detail)
	if strings.Contains(string(stored)+f.logs.String(), "PRIVATE_") {
		t.Fatal("private tool result persisted")
	}
}
