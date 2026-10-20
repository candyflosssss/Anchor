package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

// Exercise the two actual client protocols with scoped mapped-model credentials.
func TestMappedAgentsGatewayProtocols(t *testing.T) {
	for _, adapter := range []string{"claude", "dsh", "dsh-desktop"} {
		t.Run(adapter, func(t *testing.T) {
			f := setup(t)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Authorization") != "Bearer fixture-supplier-secret" {
					t.Error("supplier credential not used")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != "actual" {
					t.Error("alias not routed", body["model"])
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"fixture-response","object":"response","status":"completed","model":"actual","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"mock reply"}]}],"usage":{"input_tokens":1,"output_tokens":2}}`))
			}))
			defer upstream.Close()
			f.add(t, "p", upstream.URL, "fixture-supplier-secret")
			for _, id := range []string{"public", "unflagged"} {
				if _, err := f.store.PutModel(state.Model{ID: id, Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}, 0); err != nil {
					t.Fatal(err)
				}
			}
			exe, _ := os.Executable()
			b, err := f.store.AddBinding(state.Binding{ID: adapter, AdapterID: adapter, AdapterVersion: "1", ConfigRoot: t.TempDir(), Executable: exe, Model: "public", ModelScope: "all-mapped", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			token, err := f.store.BindingToken(b.ID)
			if err != nil {
				t.Fatal(err)
			}
			resp := request(t, "GET", f.gateway.URL+"/v1/models", token, "")
			if raw := text(t, resp); resp.StatusCode != 200 || !strings.Contains(raw, `"id":"unflagged"`) {
				t.Fatal(resp.StatusCode, raw)
			}
			path := "/v1/chat/completions"
			input := `{"model":"unflagged","messages":[{"role":"user","content":"mock"}]}`
			if adapter == "claude" {
				path = "/v1/messages"
				input = `{"model":"unflagged","max_tokens":32,"messages":[{"role":"user","content":"mock"}]}`
			}
			resp = request(t, "POST", f.gateway.URL+path, token, input)
			if raw := text(t, resp); resp.StatusCode != 200 || !strings.Contains(raw, "mock reply") || strings.Contains(raw, "fixture-supplier-secret") {
				t.Fatal(resp.StatusCode, raw)
			}
			resp = request(t, "POST", f.gateway.URL+path, token, strings.Replace(input, "unflagged", "unknown", 1))
			if resp.StatusCode != 403 || calls.Load() != 1 {
				t.Fatal("unknown mapping reached supplier", resp.StatusCode, calls.Load(), text(t, resp))
			}
			var denied struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&denied); err != nil {
				t.Fatal(err)
			}
			if denied.Error.Code != "binding_model_denied" || denied.Error.Message != "[Anchor] model is outside the binding route scope" {
				t.Fatalf("binding rejection lacks origin: %+v", denied.Error)
			}
			resp = request(t, "GET", f.control.URL+"/control/v1/status", token, "")
			if resp.StatusCode != 401 {
				t.Fatal("binding managed core", resp.StatusCode)
			}
			if _, err := f.store.EnableBinding(b.ID, b.Revision, false); err != nil {
				t.Fatal(err)
			}
			resp = request(t, "POST", f.gateway.URL+path, token, input)
			if resp.StatusCode != 401 || calls.Load() != 1 {
				t.Fatal("disabled credential accepted", resp.StatusCode, calls.Load())
			}
		})
	}
}
