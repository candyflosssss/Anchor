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

func TestGrokUsageGatewayAccounting(t *testing.T) {
	for _, upstream := range []string{"responses", "chat-completions"} {
		for _, client := range []string{"responses", "chat-completions", "anthropic"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s_to_%s/stream_%t", upstream, client, stream), func(t *testing.T) {
					f := setup(t)
					var calls atomic.Int32
					up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if r.URL.Path != protocol.Endpoint(upstream) {
							t.Errorf("unexpected upstream path %s", r.URL.Path)
						}
						var requestBody protocol.Object
						if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil || requestBody["model"] != "arbitrary-model" {
							t.Errorf("bad dispatch %+v: %v", requestBody, err)
						}
						usage := `{"input_tokens":32,"output_tokens":9,"total_tokens":135,"input_tokens_details":{"cached_tokens":4},"output_tokens_details":{"reasoning_tokens":94}}`
						response := `{"id":"r","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"PRIVATE_REPLY"}]}],"usage":` + usage + `}`
						if upstream == "chat-completions" {
							usage = `{"prompt_tokens":32,"completion_tokens":9,"total_tokens":135,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":94}}`
							response = `{"id":"r","choices":[{"message":{"content":"PRIVATE_REPLY"},"finish_reason":"stop"}],"usage":` + usage + `}`
						}
						if !stream {
							w.Header().Set("Content-Type", "application/json")
							io.WriteString(w, response)
							return
						}
						w.Header().Set("Content-Type", "text/event-stream")
						send := func(data string) { fmt.Fprintf(w, "data: %s\n\n", data) }
						if upstream == "responses" {
							send(`{"type":"response.created","response":{"id":"r"}}`)
							send(`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg"}}`)
							send(`{"type":"response.output_text.delta","output_index":0,"delta":"PRIVATE_REPLY"}`)
							send(`{"type":"response.completed","response":` + response + `}`)
						} else {
							send(`{"id":"r","choices":[{"index":0,"delta":{"content":"PRIVATE_REPLY"}}]}`)
							send(`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
							send(`{"choices":[],"usage":` + usage + `}`)
							send(`[DONE]`)
						}
					}))
					defer up.Close()
					f.add(t, "p", up.URL, "synthetic-no-log-secret")
					if err := f.store.PutKeyModelProtocol("p", 1, "default", "arbitrary-model", upstream); err != nil {
						t.Fatal(err)
					}
					snapshot := logPriceSnapshot(t, "vision-reasoner")
					_, err := f.store.PutModel(state.Model{ID: "public", Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "arbitrary-model"}}, Settings: &state.ModelSettings{Reference: &snapshot}}, 0)
					if err != nil {
						t.Fatal(err)
					}
					body := fmt.Sprintf(`{"model":"public","input":"PRIVATE_PROMPT","stream":%t}`, stream)
					if client != "responses" {
						body = fmt.Sprintf(`{"model":"public","max_tokens":256,"messages":[{"role":"user","content":"PRIVATE_PROMPT"}],"stream":%t}`, stream)
					}
					resp := request(t, "POST", f.gateway.URL+"/v1"+protocol.Endpoint(client), f.token, body)
					reply := text(t, resp)
					resp.Body.Close()
					if resp.StatusCode != 200 || !strings.Contains(reply, "PRIVATE_REPLY") || strings.Contains(reply, "event: error") || calls.Load() != 1 {
						t.Fatal(resp.StatusCode, reply, calls.Load())
					}
					if err = f.core.logs.Flush(t.Context()); err != nil {
						t.Fatal(err)
					}
					row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
					if err != nil || row.Input == nil || *row.Input != 32 || row.Output == nil || *row.Output != 103 || len(row.Attempts) != 1 || row.Attempts[0].Output == nil || *row.Attempts[0].Output != 103 {
						t.Fatalf("wrong accounted usage: %+v err=%v", row, err)
					}
					if row.Metrics.ReasoningTokens == nil || *row.Metrics.ReasoningTokens != 94 || row.Metrics.CacheReadTokens == nil || *row.Metrics.CacheReadTokens != 4 || stream && (row.Metrics.OutputTextTokens == nil || *row.Metrics.OutputTextTokens != 9) {
						t.Fatal("wrong reasoning/cache/visible token metrics", row.Metrics)
					}
					wantUSD, reason := estimateLogAttempt(referenceCost(&snapshot), protocol.Usage{Known: true, Input: 32, Output: 103, CacheRead: 4, Reasoning: 94, CacheReadKnown: true, CacheWriteKnown: true, ReasoningKnown: true})
					if reason != "" || wantUSD == nil || row.ReferenceCost == nil || row.ReferenceCost.USD == nil || row.ReferenceCost.Reason != "" || row.Attempts[0].ReferenceUSD == nil || *row.Attempts[0].ReferenceUSD != *wantUSD {
						t.Fatal("wrong reference cost", row.ReferenceCost, row.Attempts, wantUSD, reason)
					}
					if row.Evidence["translation_category"] != "" || row.Evidence["conversion_target"] != upstream || row.Evidence["translation_delivered"] != "true" {
						t.Fatal("unexpected translation failure", row.Evidence)
					}
					stored, _ := json.Marshal(row)
					if strings.Contains(string(stored)+f.logs.String(), "PRIVATE_") || strings.Contains(string(stored)+f.logs.String(), "synthetic-no-log-secret") {
						t.Fatal("private content persisted")
					}
				})
			}
		}
	}
}
