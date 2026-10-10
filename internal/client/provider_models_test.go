package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
)

type directoryTransport func(*http.Request) (*http.Response, error)

func (fn directoryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }
func TestDirectoryFailuresAreReadsAndNeverReplayed(t *testing.T) {
	for _, transportFailure := range []bool{false, true} {
		calls := 0
		c := &Client{endpoint: "http://127.0.0.1", http: &http.Client{Transport: directoryTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != "POST" || r.URL.Path != "/control/v1/providers/中文目录/discover-models" {
				t.Fatal("unexpected dispatch")
			}
			if transportFailure {
				return nil, errors.New("raw-synthetic-credential")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("not json")), Header: make(http.Header)}, nil
		})}}
		_, err := c.DiscoverProviderModels(context.Background(), catalog.DiscoverProviderModels{ProviderID: "中文目录", ExpectedRevision: 1})
		var requestError *RequestError
		if !errors.As(err, &requestError) || requestError.Unknown || calls != 1 || strings.Contains(err.Error(), "raw-synthetic-credential") {
			t.Fatal("read failure mapping", err, calls)
		}
	}
}
