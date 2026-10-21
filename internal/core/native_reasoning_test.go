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

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestGatewaySanitizesNativeReasoningBeforeFirstDispatch(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			f := setup(t)
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body struct {
					Input []map[string]json.RawMessage `json:"input"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Input) != 3 {
					t.Error("invalid dispatch", err)
					w.WriteHeader(400)
					return
				}
				if _, exists := body.Input[0]["encrypted_content"]; exists {
					w.WriteHeader(400)
					io.WriteString(w, `{"error":{"message":"encrypted reasoning content must be a string"}}`)
					return
				}
				if _, exists := body.Input[0]["id"]; exists {
					t.Error("orphan reasoning id retained")
				}
				if !strings.Contains(string(body.Input[0]["summary"]), "keep summary") || string(body.Input[1]["encrypted_content"]) != `"opaque"` || string(body.Input[2]["content"]) != `"continue"` {
					t.Error("history was lost", body.Input)
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"id":"r","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":3,"output_tokens":1}}`)
				}
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "synthetic-upstream-credential")
			f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, fmt.Sprintf(`{"model":"public","stream":%t,"input":[{"type":"reasoning","id":"rs_bad","summary":[{"type":"summary_text","text":"keep summary"}],"encrypted_content":null},{"type":"reasoning","id":"rs_good","summary":[],"encrypted_content":"opaque"},{"role":"user","content":"continue"}]}`, stream))
			body := text(t, resp)
			if resp.StatusCode != 200 || calls.Load() != 1 || !strings.Contains(body, "OK") {
				t.Fatal("first dispatch failed", resp.StatusCode, calls.Load(), body)
			}
			if err := f.core.logs.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			row, err := logClient(t, f).LogDetail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
			if err != nil || row.Status != "succeeded" || row.AttemptCount != 1 || row.Evidence["encrypted_context_recovery"] != "" || row.Input == nil || *row.Input != 3 {
				t.Fatal("sanitization incorrectly recorded as retry", row, err)
			}
		})
	}
}
