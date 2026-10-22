package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestLocalhostProviderGateway(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Host, "localhost:") || r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer synthetic-local-key" {
			t.Error("local upstream request lost host, path or authentication")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"local-response","output":[]}`)
	}))
	defer up.Close()
	base := strings.Replace(up.URL, "127.0.0.1", "localhost", 1) + "/v1"
	f.add(t, "local", base, "synthetic-local-key")
	f.model(t, state.Candidate{ProviderID: "local", Model: "up"})
	r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
	body := text(t, r)
	if r.StatusCode != http.StatusOK || !strings.Contains(body, "local-response") {
		t.Fatalf("local gateway response: %d %s", r.StatusCode, body)
	}
}
