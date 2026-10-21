package providerapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDetectionAndQueryShareBudgetAcrossAdapters(t *testing.T) {
	for _, large := range []bool{false, true} {
		t.Run(map[bool]string{false: "request count", true: "total bytes"}[large], func(t *testing.T) {
			var calls atomic.Int32
			body := "{}"
			if large {
				body = `{"padding":"` + strings.Repeat("x", 1100<<10) + `"}`
			}
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); io.WriteString(w, body) }))
			defer up.Close()
			ctx := WithRequestBudget(t.Context())
			limit := 8
			if large {
				limit = 1
			}
			for range limit {
				a := Adapter{BaseURL: up.URL}
				if _, err := a.getWithAuth(ctx, "/api/status", false, true, nil); err != nil {
					t.Fatal(err)
				}
			}
			a := Adapter{BaseURL: up.URL}
			_, err := a.getWithAuth(ctx, "/api/status", false, true, nil)
			if err == nil || err.Error() != "too_large" {
				t.Fatal("independent adapters escaped the shared limit", err)
			}
			expected := int32(8)
			if large {
				expected = 2
			}
			if calls.Load() != expected {
				t.Fatal("unexpected network requests", calls.Load())
			}
		})
	}
}
