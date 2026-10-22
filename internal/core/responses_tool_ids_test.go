package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestGatewayRepairsResponsesToolIDsOnHistoryReplay(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			f := setup(t)
			var calls atomic.Int32
			tools := []any{protocol.Object{"type": "custom", "name": "exec"}, protocol.Object{"type": "function", "name": "lookup", "parameters": protocol.Object{"type": "object"}}}
			output := []any{
				protocol.Object{"type": "custom_tool_call", "id": "call_fixture", "call_id": "call_fixture", "name": "exec", "input": "PRIVATE_CODE", "status": "completed"},
				protocol.Object{"type": "function_call", "id": "call_lookup", "call_id": "call_lookup", "name": "lookup", "arguments": "{}", "status": "completed"},
			}
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				var body struct {
					Input []protocol.Object `json:"input"`
					Tools []protocol.Object `json:"tools"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if len(body.Tools) != 2 || body.Tools[0]["name"] != "exec" || body.Tools[1]["name"] != "lookup" {
					t.Error("tool definitions lost", body.Tools)
				}
				if n == 2 {
					if len(body.Input) != 5 || body.Input[1]["id"] != "ctc_call_fixture" || body.Input[1]["call_id"] != "call_fixture" || body.Input[1]["input"] != "PRIVATE_CODE" || body.Input[2]["id"] != "fc_call_lookup" || body.Input[2]["call_id"] != "call_lookup" || body.Input[3]["id"] != "ctco_call_result" || body.Input[3]["call_id"] != "call_fixture" || body.Input[3]["output"] != "PRIVATE_RESULT" || body.Input[4]["call_id"] != "call_lookup" {
						t.Error("invalid tool history dispatched", body.Input)
						w.WriteHeader(400)
						return
					}
				}
				result := protocol.Object{"id": "resp_fixture", "status": "completed", "output": output, "usage": protocol.Object{"input_tokens": 3, "output_tokens": 2}}
				if n == 2 {
					result["output"] = []any{protocol.Object{"type": "message", "role": "assistant", "content": []any{protocol.Object{"type": "output_text", "text": "FINAL_REPLY"}}}}
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					jsonBytes, _ := json.Marshal(protocol.Object{"type": "response.completed", "response": result})
					fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
				} else {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(result)
				}
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "synthetic")
			f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
			input := []any{protocol.Object{"role": "user", "content": "PRIVATE_PROMPT"}}
			raw, _ := json.Marshal(protocol.Object{"model": "public", "stream": stream, "tools": tools, "input": input})
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, string(raw))
			first := text(t, resp)
			var result protocol.Object
			if stream {
				scanner := bufio.NewScanner(strings.NewReader(first))
				for scanner.Scan() {
					if line := scanner.Text(); strings.HasPrefix(line, "data: ") {
						var event struct {
							Response protocol.Object `json:"response"`
						}
						if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
							t.Fatal(err)
						}
						result = event.Response
					}
				}
				if err := scanner.Err(); err != nil {
					t.Fatal(err)
				}
			} else if err := json.Unmarshal([]byte(first), &result); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 200 || result == nil {
				t.Fatal("tool response failed", resp.StatusCode, first)
			}
			returned := result["output"].([]any)
			if returned[0].(map[string]any)["id"] != "call_fixture" {
				t.Fatal("native response was modified")
			}
			input = append(input, returned...)
			input = append(input, protocol.Object{"type": "custom_tool_call_output", "id": "call_result", "call_id": "call_fixture", "output": "PRIVATE_RESULT"}, protocol.Object{"type": "function_call_output", "call_id": "call_lookup", "output": "found"})
			raw, _ = json.Marshal(protocol.Object{"model": "public", "stream": stream, "tools": tools, "input": input})
			resp = request(t, "POST", f.gateway.URL+"/v1/responses", f.token, string(raw))
			final := text(t, resp)
			if resp.StatusCode != 200 || calls.Load() != 2 || !strings.Contains(final, "FINAL_REPLY") {
				t.Fatal("tool loop failed", resp.StatusCode, calls.Load(), final)
			}
			if err := f.core.logs.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			detail, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
			if err != nil || detail.Status != "succeeded" || detail.AttemptCount != 1 || detail.Evidence["encrypted_context_recovery"] != "" {
				t.Fatal("repair must not cause a retry", detail, err)
			}
			stored, _ := json.Marshal(detail)
			if strings.Contains(string(stored)+f.logs.String(), "PRIVATE_") {
				t.Fatal("tool contents leaked into logs")
			}
		})
	}
}
