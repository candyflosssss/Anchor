package modeldev

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type resolveTransport func(*http.Request) (*http.Response, error)

func (f resolveTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestResolveExactCatalogIdentity(t *testing.T) {
	raw, err := os.ReadFile("testdata/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, raw, id string
		found         bool
	}{
		{"unique", string(raw), "vision-reasoner", true},
		{"qualified", string(raw), "lab/vision-reasoner", true},
		{"no fuzzy matching", string(raw), "vision", false},
		{"ambiguous", `{"a":{"models":{"same":{}}},"b":{"models":{"same":{}}}}`, "same", false},
		{"qualified ambiguous", `{"a":{"models":{"same":{}}},"b":{"models":{"same":{}}}}`, "a/same", true},
		{"canonical owner", `{"a":{"models":{"same":{"canonical_model_id":"a/same"}}},"b":{"models":{"same":{"canonical_model_id":"a/same"}}}}`, "same", true},
		{"unannotated reseller", `{"a":{"models":{"same":{"canonical_model_id":"a/same"}}},"b":{"models":{"same":{}}}}`, "same", true},
		{"contradictory canonical", `{"a":{"models":{"same":{"canonical_model_id":"a/same"}}},"b":{"models":{"same":{"canonical_model_id":"other/same"}}}}`, "same", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := NewWithTransport(resolveTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.raw)), Header: make(http.Header)}, nil
			}))
			defer c.Close()
			v, err := c.Resolve(context.Background(), tc.id)
			if (err == nil) != tc.found {
				t.Fatalf("found=%v err=%v", tc.found, err)
			}
			if tc.found {
				if len(v.Models) != 1 || v.Stale || v.Version == "" {
					t.Fatal(v)
				}
				v.Models[0].Name = "mutated"
				again, err := c.Resolve(context.Background(), tc.id)
				if err != nil || again.Models[0].Name == "mutated" || calls != 1 {
					t.Fatal("cache escaped", again, err, calls)
				}
			}
		})
	}
}

func TestResolveRejectsStaleAndCancelledCatalog(t *testing.T) {
	c, _, fail := catalogClient(t)
	if _, err := c.Resolve(context.Background(), "vision-reasoner"); err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return time.Now().Add(2 * CacheTTL) }
	fail.Store(true)
	if _, err := c.Resolve(context.Background(), "vision-reasoner"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Resolve(ctx, "vision-reasoner"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

// Optional offline regression against a downloaded public catalog: no upstream
// inference credentials, live pricing calls or user state are involved.
func TestResolveDownloadedCatalogDefaults(t *testing.T) {
	path := os.Getenv("ANCHOR_MODELDEV_FILE")
	if path == "" {
		t.Skip("requires downloaded public catalog")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c := NewWithTransport(resolveTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
	}))
	defer c.Close()
	for _, id := range []string{"gpt-6.1-sol", "gpt-6-astra", "gpt-5.6-sol"} {
		v, err := c.Resolve(t.Context(), id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		m := v.Models[0]
		if m.ProviderID != "openai" || m.Limits.Context == nil || m.Limits.Output == nil {
			t.Fatal(id, m)
		}
		t.Logf("%s: context=%d output=%d", id, *m.Limits.Context, *m.Limits.Output)
	}
}

func TestResolveCachedDoesNotFetchAndPreservesSnapshot(t *testing.T) {
	calls := 0
	c := NewWithTransport(resolveTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"p":{"models":{"m":{"cost":{"input":1,"output":2}}}}}`)), Header: make(http.Header)}, nil
	}))
	defer c.Close()
	if _, err := c.ResolveCached("m"); !errors.Is(err, ErrUnavailable) || calls != 0 {
		t.Fatal(err, calls)
	}
	if _, err := c.Resolve(t.Context(), "m"); err != nil {
		t.Fatal(err)
	}
	m, err := c.ResolveCached("m")
	if err != nil {
		t.Fatal(err)
	}
	*m.Models[0].Pricing.Rates.Input = "99"
	again, err := c.ResolveCached("m")
	if err != nil || calls != 1 || *again.Models[0].Pricing.Rates.Input != "1" {
		t.Fatal(again, err, calls)
	}
	c.now = func() time.Time { return time.Now().Add(2 * CacheTTL) }
	if _, err := c.ResolveCached("m"); !errors.Is(err, ErrUnavailable) || calls != 1 {
		t.Fatal("cached lookup fetched expired catalog", err, calls)
	}
}
