package core

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestUpstreamDiagnosticSurvivesExhaustedCandidates(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(429)
		io.WriteString(w, `{"error":{"code":"insufficient_quota","message":"private-remote-message"}}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic-secret")
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"private-input"}`)
	if raw := text(t, resp); resp.StatusCode != 503 || strings.Contains(raw, "private") {
		t.Fatalf("%d %s", resp.StatusCode, raw)
	}
	if err := f.core.logs.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
	if err != nil || row.Evidence["upstream_error_category"] != "insufficient_quota" || row.AttemptCount != 1 || row.HTTP == nil || *row.HTTP != 429 {
		t.Fatalf("%+v %v", row, err)
	}
	raw, _ := json.Marshal(row)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "synthetic-secret") {
		t.Fatal("upstream error leaked")
	}
}

func TestUpstreamDiagnosticPreservesBodyAndOnlyRecordsKnownCategories(t *testing.T) {
	for _, tc := range []struct{ body, category string }{
		{`{"error":{"code":"insufficient_quota","message":"private-key and prompt"}}`, "insufficient_quota"},
		{`{"error":{"code":"private-code","type":"authentication_error","message":"private"}}`, "invalid_api_key"},
		{`{"error":{"code":"private-code","type":"private-type","message":"private"}}`, ""},
		{`{"error":{"code":123,"message":"private"}}`, ""},
		{"<html>private-error</html>", ""},
		{`{"error":{"code":"invalid_api_key","message":"` + strings.Repeat("x", 32<<10) + `"}}`, ""},
	} {
		t.Run(tc.category+"/"+strconv.Itoa(len(tc.body)), func(t *testing.T) {
			body := &diagnosticBody{Reader: strings.NewReader(tc.body)}
			resp := &http.Response{Body: body}
			evidence := map[string]string{}
			captureUpstreamError(resp, evidence)
			if body.read > 16<<10 {
				t.Fatalf("unbounded diagnostic read: %d", body.read)
			}
			if evidence["upstream_error_category"] != tc.category {
				t.Fatalf("evidence=%v", evidence)
			}
			encoded, _ := json.Marshal(evidence)
			if strings.Contains(string(encoded), "private") || len(evidence) > 1 {
				t.Fatalf("unsafe diagnostic: %s", encoded)
			}
			restored, err := io.ReadAll(resp.Body)
			if err != nil || string(restored) != tc.body {
				t.Fatal("body changed before recovery detector", err)
			}
			resp.Body.Close()
			if !body.closed {
				t.Fatal("underlying body not closed")
			}
		})
	}
}

type diagnosticBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *diagnosticBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (b *diagnosticBody) Close() error { b.closed = true; return nil }
