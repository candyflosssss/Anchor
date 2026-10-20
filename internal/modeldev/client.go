package modeldev

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const Endpoint = "https://models.dev/api.json"
const MaxBytes = 16 << 20
const CacheTTL = time.Hour

// ReferenceTTL bounds the age of prices used for new request estimates, even
// when an hourly refresh fails. Metadata imports still require CacheTTL freshness.
const ReferenceTTL = 3 * CacheTTL
const FetchTimeout = 30 * time.Second

// Client owns one bounded immutable catalog snapshot. Concurrent refreshes are
// coalesced; network failure retains the last snapshot with explicit staleness.
type Client struct {
	http        *http.Client
	ctx         context.Context
	cancel      context.CancelFunc
	now         func() time.Time
	mu          sync.Mutex
	snapshot    *snapshot
	inflight    chan struct{}
	failed      bool
	attempted   time.Time
	refreshWake chan struct{}
	refreshDone chan struct{}
}
type snapshot struct {
	data    map[string]providerData
	fetched time.Time
	version string
}

func New() *Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	// Only this credential-free public catalog uses standard network proxies.
	tr.Proxy = http.ProxyFromEnvironment
	// Standard gzip transfer reduces the public catalog download; the body
	// limit below applies after decompression.
	tr.DisableCompression = false
	tr.MaxConnsPerHost = 1
	tr.MaxIdleConns = 1
	tr.ResponseHeaderTimeout = FetchTimeout
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{ctx: ctx, cancel: cancel, http: &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, now: time.Now, refreshWake: make(chan struct{}, 1)}
}

// NewWithTransport injects an HTTP transport while preserving the fixed public
// endpoint, redirect policy and timeout. The client owns its idle connections.
func NewWithTransport(transport http.RoundTripper) *Client {
	c := New()
	c.http.Transport = transport
	return c
}
func (c *Client) Close() {
	c.cancel()
	c.mu.Lock()
	done := c.refreshDone
	c.mu.Unlock()
	if done != nil {
		<-done
	}
	c.mu.Lock()
	wait := c.inflight
	c.mu.Unlock()
	if wait != nil {
		<-wait
	}
	c.http.CloseIdleConnections()
}

func (c *Client) load(ctx context.Context, refresh bool) (*snapshot, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if c.ctx.Err() != nil {
		return nil, false, ErrUnavailable
	}
	c.mu.Lock()
	now := c.now()
	if c.inflight == nil {
		if !refresh && c.snapshot != nil && ((!c.failed && now.Sub(c.snapshot.fetched) < CacheTTL) || (c.failed && now.Sub(c.attempted) < time.Minute)) {
			s, failed := c.snapshot, c.failed
			c.mu.Unlock()
			return s, failed || now.Sub(s.fetched) >= CacheTTL, nil
		}
		if !refresh && c.snapshot == nil && c.failed && now.Sub(c.attempted) < time.Minute {
			c.mu.Unlock()
			return nil, false, ErrUnavailable
		}
		c.inflight = make(chan struct{})
		// A cancelled reader only stops waiting. It must not cancel a shared fetch
		// or poison other readers' cache. Close and FetchTimeout bound that work.
		go c.refresh(c.inflight)
	}
	wait := c.inflight
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-wait:
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	c.mu.Lock()
	s, failed := c.snapshot, c.failed
	c.mu.Unlock()
	if s == nil {
		return nil, false, ErrUnavailable
	}
	return s, failed || c.now().Sub(s.fetched) >= CacheTTL, nil
}
func (c *Client) refresh(wait chan struct{}) {
	s, err := c.fetch(c.ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attempted = c.now()
	c.failed = err != nil
	if err == nil {
		c.snapshot = s
	}
	close(wait)
	c.inflight = nil
}

func (c *Client) fetch(ctx context.Context) (*snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, FetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Endpoint, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > MaxBytes {
		return nil, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil || len(raw) > MaxBytes {
		return nil, ErrUnavailable
	}
	data, err := parse(raw)
	if err != nil {
		return nil, ErrUnavailable
	}
	hash := sha256.Sum256(raw)
	return &snapshot{data: data, fetched: c.now(), version: hex.EncodeToString(hash[:])}, nil
}
func (c *Client) Query(ctx context.Context, q ModelCatalogQuery) (ModelCatalog, error) {
	if q.Validate() != nil {
		return ModelCatalog{}, ErrInvalid
	}
	s, stale, err := c.load(ctx, q.Refresh)
	if err != nil {
		return ModelCatalog{}, err
	}
	out := ModelCatalog{Source: "models.dev", FetchedAt: s.fetched.Unix(), ExpiresAt: s.fetched.Add(CacheTTL).Unix(), Version: s.version, Stale: stale, Providers: []CatalogProvider{}, Models: []CatalogModel{}}
	if stale {
		out.Failure = "refresh_failed"
	}
	if q.ProviderID == "" {
		for _, p := range s.data {
			out.Providers = append(out.Providers, p.Info)
		}
		slices.SortFunc(out.Providers, func(a, b CatalogProvider) int { return strings.Compare(a.ID, b.ID) })
		out.Total = len(out.Providers)
	} else {
		p, ok := s.data[q.ProviderID]
		if !ok {
			return ModelCatalog{}, ErrNotFound
		}
		if q.ModelID != "" {
			m, ok := p.Models[q.ModelID]
			if !ok {
				return ModelCatalog{}, ErrNotFound
			}
			out.Models = append(out.Models, m)
			out.Total = 1
		} else {
			search := strings.ToLower(q.Search)
			for _, m := range p.Models {
				if strings.Contains(strings.ToLower(m.ID), search) || strings.Contains(strings.ToLower(m.Name), search) {
					out.Models = append(out.Models, m)
				}
			}
			slices.SortFunc(out.Models, func(a, b CatalogModel) int { return strings.Compare(a.ID, b.ID) })
			out.Total = len(out.Models)
			limit := q.Limit
			if limit == 0 {
				limit = 50
			}
			if len(out.Models) > limit {
				out.Models = out.Models[:limit]
				out.Truncated = true
			}
		}
	}
	// Callers may edit defaults in memory; never expose mutable cache references.
	raw, err := json.Marshal(out)
	if err != nil {
		return ModelCatalog{}, ErrInvalid
	}
	var copy ModelCatalog
	if json.Unmarshal(raw, &copy) != nil {
		return ModelCatalog{}, ErrInvalid
	}
	return copy, nil
}
func (c *Client) Estimate(ctx context.Context, in CatalogEstimateInput) (CatalogEstimate, error) {
	if in.Validate() != nil {
		return CatalogEstimate{}, ErrInvalid
	}
	catalog, err := c.Query(ctx, in.Query)
	if err != nil {
		return CatalogEstimate{}, err
	}
	out, err := Estimate(catalog.Models[0], in.Usage)
	out.Catalog = catalog
	return out, err
}
