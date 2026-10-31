package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestAnthropicGatewayConfiguredOutputBudget(t *testing.T) {
	for _, client := range []string{"responses", "chat-completions", "anthropic"} {
		for _, stream := range []bool{false, true} {
			for _, tc := range []struct {
				name                                       string
				maximum, defaultBudget, clientBudget, want int64
				source                                     string
				limited                                    bool
			}{
				{"declared maximum", 128000, 0, 0, 128000, "model_limit", false},
				{"configured default", 128000, 16000, 0, 16000, "model_default", false},
				{"client smaller", 128000, 16000, 1024, 1024, "client", true},
				{"client above declaration", 128000, 0, 256000, 256000, "client", false},
				{"legacy without declarations", 0, 0, 0, 4096, "legacy_fallback", true},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", client, stream, tc.name), func(t *testing.T) {
					f := setup(t)
					var calls atomic.Int32
					up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						var body protocol.Object
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
						}
						if body["max_tokens"] != float64(tc.want) || r.URL.Path != "/messages" {
							t.Errorf("budget=%v expected=%d path=%s", body["max_tokens"], tc.want, r.URL.Path)
						}
						finish, output := "end_turn", int64(5000)
						if tc.limited {
							finish, output = "max_tokens", tc.want
						}
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							events := []protocol.Object{
								{"type": "message_start", "message": protocol.Object{"id": "r", "usage": protocol.Object{"input_tokens": 2, "output_tokens": 0}}},
								{"type": "content_block_start", "index": 0, "content_block": protocol.Object{"type": "text", "text": ""}},
								{"type": "content_block_delta", "index": 0, "delta": protocol.Object{"type": "text_delta", "text": "VISIBLE_REPLY"}},
								{"type": "content_block_stop", "index": 0},
								{"type": "message_delta", "delta": protocol.Object{"stop_reason": finish}, "usage": protocol.Object{"output_tokens": output}},
								{"type": "message_stop"},
							}
							for _, event := range events {
								raw, _ := json.Marshal(event)
								fmt.Fprintf(w, "data: %s\n\n", raw)
							}
							return
						}
						w.Header().Set("Content-Type", "application/json")
						io.WriteString(w, fmt.Sprintf(`{"id":"r","content":[{"type":"text","text":"VISIBLE_REPLY"}],"stop_reason":%q,"usage":{"input_tokens":2,"output_tokens":%d}}`, finish, output))
					}))
					defer up.Close()
					f.add(t, "p", up.URL, "synthetic")
					if err := f.store.PutKeyModelProtocol("p", 1, "default", "up", "anthropic"); err != nil {
						t.Fatal(err)
					}
					f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
					if tc.maximum > 0 || tc.defaultBudget > 0 {
						m, err := f.store.Model("public")
						if err != nil {
							t.Fatal(err)
						}
						m.Settings = &state.ModelSettings{}
						if tc.maximum > 0 {
							maximum := tc.maximum
							m.Settings.Configuration.Limits.Output = &maximum
						}
						if tc.defaultBudget > 0 {
							budget := tc.defaultBudget
							m.Settings.Configuration.DefaultOutputTokens = &budget
						}
						if _, err = f.store.PutModel(m, m.Revision); err != nil {
							t.Fatal(err)
						}
					}
					body := protocol.Object{"model": "public", "stream": stream}
					field := "max_output_tokens"
					if client == "responses" {
						body["input"] = "hello"
					} else {
						body["messages"] = []any{protocol.Object{"role": "user", "content": "hello"}}
						field = "max_tokens"
					}
					if tc.clientBudget > 0 {
						body[field] = tc.clientBudget
					}
					raw, _ := json.Marshal(body)
					resp := request(t, "POST", f.gateway.URL+"/v1"+protocol.Endpoint(client), f.token, string(raw))
					wire := text(t, resp)
					if resp.StatusCode != 200 || calls.Load() != 1 || !strings.Contains(wire, "VISIBLE_REPLY") {
						t.Fatal(resp.StatusCode, calls.Load(), wire)
					}
					if client == "responses" && tc.limited && !strings.Contains(wire, "max_output_tokens") {
						t.Fatal("truncation hidden", wire)
					}
					if err := f.core.logs.Flush(t.Context()); err != nil {
						t.Fatal(err)
					}
					detail, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
					if err != nil {
						t.Fatal(err)
					}
					status := "completed"
					if tc.limited {
						status = "incomplete"
					}
					if detail.GenerationStatus != status || detail.AttemptCount != 1 || detail.Evidence["translation_max_output_tokens"] != strconv.FormatInt(tc.want, 10) || detail.Evidence["translation_output_budget_source"] != tc.source {
						t.Fatal(detail.GenerationStatus, detail.AttemptCount, detail.Evidence)
					}
				})
			}
		}
	}
}
