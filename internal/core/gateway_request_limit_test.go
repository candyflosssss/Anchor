package core

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGatewayRequestBodyLimit(t *testing.T) {
	const limit = 32 << 20
	if MaxRequestBytes != limit {
		t.Fatalf("gateway request limit = %d, want %d", MaxRequestBytes, limit)
	}
	for _, endpoint := range []struct {
		name, path, payload string
	}{
		{"responses", "/v1/responses", `{"model":"unknown","input":"hello"}`},
		{"chat", "/v1/chat/completions", `{"model":"unknown","messages":[{"role":"user","content":"hello"}]}`},
		{"messages", "/v1/messages", `{"model":"unknown","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			f := setup(t)
			client := &http.Client{Timeout: 10 * time.Second}
			t.Cleanup(client.CloseIdleConnections)
			for _, tc := range []struct {
				name       string
				size       int64
				chunked    bool
				wantStatus int
				wantCode   string
			}{
				{"above_old_limit", (8 << 20) + 1, false, http.StatusNotFound, "unknown_model"},
				{"at_limit", limit, false, http.StatusNotFound, "unknown_model"},
				{"above_limit", limit + 1, false, http.StatusRequestEntityTooLarge, "request_too_large"},
				{"above_limit_chunked", limit + 1, true, http.StatusRequestEntityTooLarge, "request_too_large"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					// Valid JSON padded with whitespace tests the byte boundary without
					// sending model requests or allocating a second full-size test body.
					body := io.MultiReader(strings.NewReader(endpoint.payload), io.LimitReader(requestBodySpaces{}, tc.size-int64(len(endpoint.payload))))
					req, err := http.NewRequest(http.MethodPost, f.gateway.URL+endpoint.path, body)
					if err != nil {
						t.Fatal(err)
					}
					req.ContentLength = tc.size
					if tc.chunked {
						req.ContentLength = -1
					}
					req.Header.Set("Authorization", "Bearer "+f.token)
					req.Header.Set("Content-Type", "application/json")
					resp, err := client.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer resp.Body.Close()
					var result struct {
						Error struct {
							Code    string `json:"code"`
							Message string `json:"message"`
						} `json:"error"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
						t.Fatal(err)
					}
					if resp.StatusCode != tc.wantStatus || result.Error.Code != tc.wantCode {
						t.Fatalf("status=%d error=%+v, want status=%d code=%s", resp.StatusCode, result.Error, tc.wantStatus, tc.wantCode)
					}
					if tc.wantStatus == http.StatusRequestEntityTooLarge && !strings.Contains(result.Error.Message, "32 MiB") {
						t.Fatalf("oversize error must name the limit: %q", result.Error.Message)
					}
				})
			}
		})
	}
}

type requestBodySpaces struct{}

func (requestBodySpaces) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}
