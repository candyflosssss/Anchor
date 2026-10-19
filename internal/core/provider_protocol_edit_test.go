package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestProviderProtocolEditChangesActualUpstreamRequests(t *testing.T) {
	for _, channel := range []string{"responses", "chat-completions", "anthropic"} {
		t.Run(channel, func(t *testing.T) {
			f := setup(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1"+protocol.Endpoint(channel) {
					t.Errorf("request channel not applied: %s", r.URL.Path)
				}
				if channel == "anthropic" {
					if r.Header.Get("x-api-key") != "synthetic-channel" || r.Header.Get("Authorization") != "" {
						t.Error("incorrect Anthropic authentication")
					}
				} else if r.Header.Get("Authorization") != "Bearer synthetic-channel" || r.Header.Get("x-api-key") != "" {
					t.Error("incorrect OpenAI authentication")
				}
				w.Header().Set("Content-Type", "application/json")
				switch channel {
				case "responses":
					io.WriteString(w, `{"id":"r","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":3,"output_tokens":1}}`)
				case "chat-completions":
					io.WriteString(w, `{"id":"r","choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
				case "anthropic":
					io.WriteString(w, `{"id":"r","content":[{"type":"text","text":"OK"}],"usage":{"input_tokens":3,"output_tokens":1},"stop_reason":"end_turn"}`)
				}
			}))
			defer upstream.Close()
			f.add(t, "p", upstream.URL+"/v1", "synthetic-channel")
			// Start with an independently configured Key; an explicit provider edit must replace it.
			if err := f.store.MutateProvider("p", 1, "key-put", "default", "", "", true, false, "anthropic"); err != nil {
				t.Fatal(err)
			}
			p, err := f.store.Provider("p")
			if err != nil {
				t.Fatal(err)
			}
			p.Protocol = channel
			if _, err = f.store.PutProvider(p, p.Revision, nil); err != nil {
				t.Fatal(err)
			}
			f.model(t, state.Candidate{ProviderID: "p", Model: "actual/model"})
			response := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
			raw := text(t, response)
			if response.StatusCode != http.StatusOK || !strings.Contains(raw, "OK") {
				t.Fatal(response.StatusCode, raw)
			}
		})
	}
}
