package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestClaudeGatewayNativeHeadersAndCredentials(t *testing.T) {
	for _, native := range []bool{true, false} {
		t.Run(map[bool]string{true: "native", false: "cross"}[native], func(t *testing.T) {
			f := setup(t)
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if native {
					if r.Header.Get("anthropic-version") != "2023-06-01" || !reflect.DeepEqual(r.Header.Values("anthropic-beta"), []string{"first-beta", "second-beta"}) {
						t.Error("native metadata lost")
					}
					if r.Header.Get("x-api-key") != "fixture-upstream" || r.Header.Get("Authorization") != "" {
						t.Error("wrong native authentication")
					}
				} else if r.Header.Get("anthropic-version") != "" || r.Header.Get("anthropic-beta") != "" || r.Header.Get("Authorization") != "Bearer fixture-upstream" {
					t.Error("cross protocol header or auth leak")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != "actual" {
					t.Error("alias not mapped")
				}
				w.Header().Set("Content-Type", "application/json")
				if native {
					if body["thinking"] == nil || !strings.Contains(string(mustJSON(body)), "cache_control") {
						t.Error("native fields lost")
					}
					_, _ = w.Write([]byte(`{"id":"reply","type":"message","role":"assistant","model":"actual","content":[{"type":"text","text":"reply"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
				} else {
					_, _ = w.Write([]byte(`{"id":"reply","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"reply"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`))
				}
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "fixture-upstream")
			if native {
				if err := f.store.PutKeyModelProtocol("p", 1, "default", "actual", "anthropic"); err != nil {
					t.Fatal(err)
				}
			}
			f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
			body := `{"model":"public","max_tokens":2048,"messages":[{"role":"user","content":"hello"}]}`
			if native {
				body = `{"model":"public","max_tokens":2048,"thinking":{"type":"enabled","budget_tokens":1024},"messages":[{"role":"user","content":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"}}]}]}`
			}
			req, err := http.NewRequest("POST", f.gateway.URL+"/v1/messages", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+f.token)
			req.Header.Set("x-api-key", f.token)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("anthropic-version", "2023-06-01")
			req.Header.Add("anthropic-beta", "first-beta")
			req.Header.Add("anthropic-beta", "second-beta")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if raw := text(t, resp); resp.StatusCode != 200 || !strings.Contains(raw, "reply") {
				t.Fatal(resp.StatusCode, raw)
			}
		})
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
