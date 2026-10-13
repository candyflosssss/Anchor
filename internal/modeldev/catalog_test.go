package modeldev

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (fn transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }
func fixture(t *testing.T) string {
	t.Helper()
	b, e := os.ReadFile("testdata/catalog.json")
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func catalogClient(t *testing.T) (*Client, *atomic.Int32, *atomic.Bool) {
	t.Helper()
	raw := fixture(t)
	calls := new(atomic.Int32)
	fail := new(atomic.Bool)
	c := NewWithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.String() != Endpoint || r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected public fetch", r.URL)
		}
		if fail.Load() {
			return nil, errors.New("synthetic-secret")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw)), Header: make(http.Header)}, nil
	}))
	t.Cleanup(c.Close)
	return c, calls, fail
}
func TestMetadataAndUnknown(t *testing.T) {
	raw := fixture(t)
	m, e := ParseModel([]byte(raw), "lab", "vision-reasoner")
	if e != nil {
		t.Fatal(e)
	}
	if m.Capabilities.Vision == nil || !*m.Capabilities.Vision || *m.Capabilities.StructuredOutput || *m.Limits.Context != 400000 || len(m.ReasoningOptions) != 3 || m.ReasoningOptions[0].Values[0] != nil || *m.ReasoningOptions[1].Min != -1 {
		t.Fatal(m)
	}
	if len(m.Pricing.Tiers) != 1 || m.Pricing.Tiers[0].AboveTokens != 250000 || *m.Pricing.Rates.Input != "2.5" {
		t.Fatal(m.Pricing)
	}
	text, e := ParseModel([]byte(raw), "lab", "text-only")
	if e != nil || text.Limits.Context != nil || text.Limits.Output != nil || text.Capabilities.Vision == nil || *text.Capabilities.Vision || *text.Pricing.Rates.Input != "0" {
		t.Fatal(text, e)
	}
	unknown, e := ParseModel([]byte(raw), "other", "unknown")
	if e != nil || unknown.Capabilities.Vision != nil || unknown.Capabilities.ToolCall != nil || unknown.Pricing != nil || len(unknown.ReasoningOptions) != 0 {
		t.Fatal(unknown, e)
	}
}
func TestRejectMalformedData(t *testing.T) {
	for _, s := range []string{`{}`, `{"p":{"models":{"m":{"cost":{"input":1e-999999999,"output":1}}}}}`, `{"p":{"models":{"m":{"cost":{"input":-1,"output":1}}}}}`, `{"p":{"models":{"m":{"limit":{"context":-1}}}}}`, `{"p":{"models":{"m":{"reasoning_options":[{"type":"effort","values":["ultra"]}]}}}}`, `{"p":{"models":{"m":{"cost":{"tiers":[{"tier":{"size":1}},{"tier":{"size":1}}]}}}}}`} {
		if _, err := parse([]byte(s)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted malformed %s: %v", s, err)
		}
	}
	for _, s := range []string{"1/2", "-1", "1e999999999", "1e-999999999", "1000001", "NaN"} {
		if _, ok := parseRate(s); ok {
			t.Fatal(s)
		}
	}
}
func TestQueryCacheRefreshAndIsolation(t *testing.T) {
	c, calls, fail := catalogClient(t)
	now := time.Date(2026, 10, 13, 1, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	ctx := context.Background()
	providers, e := c.Query(ctx, ModelCatalogQuery{})
	if e != nil || len(providers.Providers) != 2 || providers.Providers[0].ID != "lab" || len(providers.Version) != 64 {
		t.Fatal(providers, e)
	}
	list, e := c.Query(ctx, ModelCatalogQuery{ProviderID: "lab", Limit: 1})
	if e != nil || !list.Truncated || list.Total != 2 || list.Models[0].ID != "text-only" {
		t.Fatal(list, e)
	}
	q := ModelCatalogQuery{ProviderID: "lab", ModelID: "vision-reasoner"}
	first, e := c.Query(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	*first.Models[0].Capabilities.Vision = false
	*first.Models[0].Pricing.Rates.Input = "999"
	again, _ := c.Query(ctx, q)
	if !*again.Models[0].Capabilities.Vision || *again.Models[0].Pricing.Rates.Input != "2.5" || calls.Load() != 1 {
		t.Fatal("cache mutation", again, calls.Load())
	}
	search, _ := c.Query(ctx, ModelCatalogQuery{ProviderID: "lab", Search: "VISION"})
	if search.Total != 1 {
		t.Fatal(search)
	}
	if _, e := c.Query(ctx, ModelCatalogQuery{ProviderID: "lab", ModelID: "Vision Reasoner"}); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	fail.Store(true)
	q.Refresh = true
	stale, e := c.Query(ctx, q)
	if e != nil || !stale.Stale || stale.Failure != "refresh_failed" || stale.Version != again.Version {
		t.Fatal(stale, e)
	}
	q.Refresh = false
	_, _ = c.Query(ctx, q)
	if calls.Load() != 2 {
		t.Fatal("backoff", calls.Load())
	}
	now = now.Add(61 * time.Second)
	fail.Store(false)
	fresh, e := c.Query(ctx, q)
	if e != nil || fresh.Stale || calls.Load() != 3 {
		t.Fatal("failed refresh retry", fresh, e, calls.Load())
	}
	now = now.Add(CacheTTL)
	_, _ = c.Query(ctx, q)
	if calls.Load() != 4 {
		t.Fatal("TTL", calls.Load())
	}
}
func TestColdFailureAndCancelledContext(t *testing.T) {
	c, calls, fail := catalogClient(t)
	fail.Store(true)
	for i := 0; i < 2; i++ {
		if _, e := c.Query(context.Background(), ModelCatalogQuery{}); !errors.Is(e, ErrUnavailable) || strings.Contains(e.Error(), "secret") {
			t.Fatal(e)
		}
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.Query(ctx, ModelCatalogQuery{Refresh: true}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestConcurrentRefreshCoalesced(t *testing.T) {
	raw := fixture(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	c := NewWithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw)), Header: make(http.Header)}, nil
	}))
	defer c.Close()
	done := make(chan error, 1)
	go func() { _, e := c.Query(context.Background(), ModelCatalogQuery{Refresh: true}); done <- e }()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := c.Query(ctx, ModelCatalogQuery{Refresh: true}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := c.Query(context.Background(), ModelCatalogQuery{})
			if e != nil {
				t.Error(e)
			}
		}()
	}
	close(release)
	wg.Wait()
	if e := <-done; e != nil || calls.Load() != 1 {
		t.Fatal(e, calls.Load())
	}
}
func TestFetchResponseBoundsAndRedirects(t *testing.T) {
	for _, status := range []int{302, 429, 500} {
		c := NewWithTransport(transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://unexpected.invalid/"}}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
		}))
		_, e := c.Query(context.Background(), ModelCatalogQuery{})
		c.Close()
		if !errors.Is(e, ErrUnavailable) {
			t.Fatal(status, e)
		}
	}
	c := NewWithTransport(transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: MaxBytes + 1, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	}))
	defer c.Close()
	if _, e := c.Query(context.Background(), ModelCatalogQuery{}); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
}

// Explicitly opt in; default tests are offline and never call inference APIs.
func TestLiveCatalogCompatibility(t *testing.T) {
	if os.Getenv("ANCHOR_MODELDEV_LIVE") != "1" {
		t.Skip("opt-in public catalog compatibility check")
	}
	c := New()
	defer c.Close()
	out, e := c.Query(context.Background(), ModelCatalogQuery{Refresh: true})
	if e != nil || out.Total == 0 || out.Stale {
		t.Fatal(out, e)
	}
	t.Logf("providers=%d version=%s", out.Total, out.Version)
}

func TestSharedDesktopCatalogFixture(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/model-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected struct {
		Catalog  ModelCatalog    `json:"catalog"`
		Estimate CatalogEstimate `json:"estimate"`
	}
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	c, _, _ := catalogClient(t)
	c.now = func() time.Time { return time.Unix(expected.Catalog.FetchedAt, 0) }
	out, err := c.Query(context.Background(), ModelCatalogQuery{ProviderID: "lab", ModelID: "vision-reasoner"})
	if err != nil || !reflect.DeepEqual(out, expected.Catalog) {
		t.Fatalf("catalog fixture drift: %+v %v", out, err)
	}
	estimate, err := c.Estimate(context.Background(), CatalogEstimateInput{Query: ModelCatalogQuery{ProviderID: "lab", ModelID: "vision-reasoner"}, Usage: expected.Estimate.Usage})
	if err != nil || !reflect.DeepEqual(estimate, expected.Estimate) {
		t.Fatalf("estimate fixture drift: %+v %v", estimate, err)
	}
}

func TestCatalogFileCompatibility(t *testing.T) {
	path := os.Getenv("ANCHOR_MODELDEV_FILE")
	if path == "" {
		t.Skip("opt-in downloaded public catalog compatibility check")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := parse(raw)
	if err != nil {
		var providers map[string]map[string]json.RawMessage
		if json.Unmarshal(raw, &providers) == nil {
			for pid, p := range providers {
				one, _ := json.Marshal(map[string]any{pid: p})
				if _, e := parse(one); e == nil {
					continue
				}
				var models map[string]json.RawMessage
				_ = json.Unmarshal(p["models"], &models)
				for mid, m := range models {
					single, _ := json.Marshal(map[string]any{pid: map[string]any{"name": "Diagnostic", "models": map[string]json.RawMessage{mid: m}}})
					if _, e := parse(single); e != nil {
						t.Errorf("unsupported public catalog provider=%s model=%s", pid, mid)
					}
				}
			}
		}
		t.Fatal(err)
	}
	total := 0
	for _, p := range data {
		total += len(p.Models)
	}
	t.Logf("bytes=%d providers=%d models=%d", len(raw), len(data), total)
}

func TestDisplayNameWhitespaceDoesNotRejectPublicCatalog(t *testing.T) {
	m, err := ParseModel([]byte(`{"p":{"name":" Provider ","models":{"m":{"name":" Model ","limit":{"context":128000,"output":2048},"cost":{"input":0.1,"output":0.2}}}}}`), "p", "m")
	if err != nil || m.Name != "Model" {
		t.Fatal(m, err)
	}
	if _, err := parse([]byte(`{"p ":{"models":{"m":{}}}}`)); !errors.Is(err, ErrInvalid) {
		t.Fatal("model/provider IDs must remain exact", err)
	}
}
func TestCancelledReaderDoesNotCancelSharedDownload(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	raw := fixture(t)
	c := NewWithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw)), Header: make(http.Header)}, nil
	}))
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := c.Query(ctx, ModelCatalogQuery{}); done <- err }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(release)
	out, err := c.Query(context.Background(), ModelCatalogQuery{})
	if err != nil || out.Stale || out.Total != 2 {
		t.Fatal(out, err)
	}
}
