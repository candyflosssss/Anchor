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

func TestAnthropicGatewayOpenAIOptions(t *testing.T) {
	for _, client := range []string{"responses", "chat-completions"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", client, stream), func(t *testing.T) {
				f := setup(t)
				var calls atomic.Int32
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var m protocol.Object
					if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
						t.Error(err)
					}
					body, _ := json.Marshal(m)
					thinking, _ := m["thinking"].(map[string]any)
					if r.URL.Path != "/messages" || m["model"] != "claude-up" || thinking["type"] != "disabled" || strings.Contains(string(body), "PRIVATE_METADATA") || strings.Contains(string(body), "encrypted_content") {
						t.Errorf("invalid converted request: %s %s", r.URL.Path, body)
					}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						for _, event := range []string{
							`{"type":"message_start","message":{"id":"r","usage":{"input_tokens":2,"output_tokens":0}}}`,
							`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
							`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"FINAL_REPLY"}}`,
							`{"type":"content_block_stop","index":0}`,
							`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
							`{"type":"message_stop"}`,
						} {
							fmt.Fprintf(w, "data: %s\n\n", event)
							w.(http.Flusher).Flush()
						}
						return
					}
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"id":"r","stop_reason":"end_turn","content":[{"type":"text","text":"FINAL_REPLY"}],"usage":{"input_tokens":2,"output_tokens":3}}`)
				}))
				defer up.Close()
				f.add(t, "p", up.URL, "synthetic")
				if err := f.store.PutKeyModelProtocol("p", 1, "default", "claude-up", "anthropic"); err != nil {
					t.Fatal(err)
				}
				f.model(t, state.Candidate{ProviderID: "p", Model: "claude-up"})
				m := protocol.Object{"model": "public", "stream": stream}
				if client == "responses" {
					m["input"] = []any{protocol.Object{"type": "reasoning", "summary": []any{}, "encrypted_content": nil}, protocol.Object{"role": "user", "content": "PRIVATE_PROMPT"}}
					m["client_metadata"] = protocol.Object{"private-key": "PRIVATE_METADATA"}
					m["stream_options"] = protocol.Object{"reasoning_summary_delivery": "sequential_cutoff"}
					m["include"] = []any{"reasoning.encrypted_content"}
					m["text"] = protocol.Object{"format": protocol.Object{"type": "text"}}
					m["reasoning"] = protocol.Object{"effort": "none", "summary": "auto"}
				} else {
					m["messages"] = []any{protocol.Object{"role": "user", "content": "PRIVATE_PROMPT"}}
					m["response_format"] = protocol.Object{"type": "text"}
					m["reasoning_effort"] = "none"
				}
				raw, _ := json.Marshal(m)
				resp := request(t, "POST", f.gateway.URL+"/v1"+protocol.Endpoint(client), f.token, string(raw))
				wire := text(t, resp)
				if resp.StatusCode != 200 || !strings.Contains(wire, "FINAL_REPLY") || calls.Load() != 1 {
					t.Fatal(resp.StatusCode, calls.Load(), wire)
				}
				f.core.logs.Flush(t.Context())
				detail, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
				if err != nil {
					t.Fatal(err)
				}
				if client == "responses" && (detail.Evidence["translation_compatibility"] != "degraded" || !strings.Contains(detail.Evidence["translation_degradations"], "client_metadata_omitted")) {
					t.Fatal(detail.Evidence)
				}
				stored, _ := json.Marshal(detail)
				if strings.Contains(string(stored)+f.logs.String(), "PRIVATE_") {
					t.Fatal("private content persisted")
				}
				// A real reasoning control remains a clear local rejection, with
				// no HTTP dispatch or silent weakening of the requested effort.
				if client == "responses" {
					m["reasoning"] = protocol.Object{"effort": "high"}
				} else {
					m["reasoning_effort"] = "high"
				}
				raw, _ = json.Marshal(m)
				resp = request(t, "POST", f.gateway.URL+"/v1"+protocol.Endpoint(client), f.token, string(raw))
				wire = text(t, resp)
				if resp.StatusCode == 200 || !strings.Contains(wire, "reasoning_options") || calls.Load() != 1 {
					t.Fatal(resp.StatusCode, calls.Load(), wire)
				}
			})
		}
	}
}
