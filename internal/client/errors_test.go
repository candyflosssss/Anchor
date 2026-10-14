package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestStructuredMutationErrorsNeverReplayOrLeak(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		unknown bool
		code    string
	}{
		{401, `{"error":{"code":"unauthorized","message":"secret-details"}}`, false, "unauthorized"},
		{409, `{"error":{"code":"conflict"}}`, false, "conflict"},
		{500, `{"error":{"code":"internal_error","message":"secret-details"}}`, true, "internal_error"},
		{400, `{"error":{"code":"中文错误","message":"secret-details"}}`, false, "request_failed"},
		{200, `not-json-secret-details`, true, "invalid_response"},
		{200, `{"accepted":false}`, true, "invalid_response"},
		{200, strings.Repeat("x", (1<<20)+1), true, "invalid_response"},
	} {
		t.Run(fmt.Sprintf("%d-%d", tc.status, len(tc.body)), func(t *testing.T) {
			var writes atomic.Int32
			web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/control/v1/instance" {
					io.WriteString(w, `{"instance_id":"test","api_version":1}`)
					return
				}
				if r.Method != "POST" || r.URL.Path != "/control/v1/shutdown" || r.Header.Get("X-Anchor-Instance") != "test" {
					t.Error("unexpected request")
				}
				writes.Add(1)
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer web.Close()
			cl, _, err := Connect(context.Background(), state.Context{Name: "test", Endpoint: web.URL, InstanceID: "test", CredentialRef: "local-owner"}, "fixture-only")
			if err != nil {
				t.Fatal(err)
			}
			defer cl.Close()
			err = cl.Stop(context.Background())
			var request *RequestError
			if !errors.As(err, &request) || request.Unknown != tc.unknown || request.Code != tc.code {
				t.Fatalf("classification: %v", err)
			}
			if writes.Load() != 1 || strings.Contains(err.Error(), "secret-details") {
				t.Fatal("write replay or leaked text")
			}
		})
	}
}
