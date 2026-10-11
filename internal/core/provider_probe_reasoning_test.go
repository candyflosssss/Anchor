package core

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeResponsesReasoning(t *testing.T) {
	for _, tc := range []struct {
		name, body, failure string
	}{
		{"completed", `{"id":"r","status":"completed","output":[{"type":"reasoning","id":"rs","summary":[]},{"type":"message","id":"msg","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":3,"output_tokens":16}}`, ""},
		{"token-limited", `{"id":"r","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"reasoning","id":"rs","summary":[]}],"usage":{"input_tokens":3,"output_tokens":16}}`, ""},
		{"failed", `{"id":"r","status":"failed","output":[{"type":"reasoning","id":"rs","summary":[]}],"error":{"message":"private upstream detail"}}`, "invalid_response"},
		{"invalid-json", `<html>upstream proxy</html>`, "invalid_response"},
		{"missing-id", `{"output":[{"type":"reasoning","id":"rs","summary":[]}]}`, "invalid_response"},
		{"unsupported-output", `{"id":"r","output":[{"type":"reasoning","id":"rs","summary":[]},{"type":"unknown"}]}`, "invalid_response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			calls := 0
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.Method != "POST" || r.URL.Path != "/v1/responses" || body["input"] != "Reply OK." || body["model"] != "reasoning-model" || body["max_output_tokens"] != float64(16) || body["store"] != false {
					t.Errorf("unexpected probe request: %s %s %v", r.Method, r.URL.Path, body)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, tc.body)
			}))
			defer up.Close()
			f.add(t, "p", up.URL+"/v1", "synthetic-key")
			p, err := f.store.Provider("p")
			if err != nil {
				t.Fatal(err)
			}
			probe, err := f.core.probe(t.Context(), p, "default", "reasoning-model")
			if err != nil {
				t.Fatal(err)
			}
			if probe.Failure != tc.failure || probe.HTTPStatus != 200 || probe.KeyID != "default" || probe.Revision != p.Revision || calls != 1 {
				t.Fatalf("probe=%+v calls=%d; want failure=%q", probe, calls, tc.failure)
			}
			w, err := f.core.workspace("p")
			if err != nil || len(w.Keys) != 1 || len(w.Usage) != 1 {
				t.Fatalf("workspace=%+v err=%v", w, err)
			}
			if w.Keys[0].Test == nil || w.Keys[0].Test.Failure != tc.failure || w.Usage[0].Requests != 1 {
				t.Fatalf("probe result not recorded: %+v", w)
			}
			if tc.failure == "" && (w.Keys[0].State != "ready" || w.Usage[0].Failures != 0 || w.Usage[0].Known != 1 || w.Usage[0].InputTokens != 3 || w.Usage[0].OutputTokens != 16) {
				t.Fatalf("successful reasoning response marked unhealthy or usage lost: %+v", w)
			}
			if tc.failure != "" && w.Usage[0].Failures != 1 {
				t.Fatalf("invalid response counted as successful: %+v", w.Usage[0])
			}
		})
	}
}
