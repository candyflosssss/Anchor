package egress

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"sync"

	"github.com/candyflosssss/Anchor/internal/state"
)

type routeKey struct{}
type route struct{ provider, key string }

func WithRoute(ctx context.Context, provider, key string) context.Context {
	return context.WithValue(ctx, routeKey{}, route{provider, key})
}
func WithKeyRoute(ctx context.Context, provider, key string, hasSecret bool) context.Context {
	if !hasSecret {
		key = ""
	}
	return WithRoute(ctx, provider, key)
}
func DefaultRoute(ctx context.Context, provider string) context.Context {
	if _, ok := ctx.Value(routeKey{}).(route); ok {
		return ctx
	}
	return WithRoute(ctx, provider, "")
}

type entry struct {
	transport *http.Transport
	engine    *engine
	refs      int
	retired   bool
}
type Manager struct {
	store   *state.Store
	base    *http.Transport
	mu      sync.Mutex
	entries map[string]*entry
	closed  bool
}

func New(store *state.Store, base *http.Transport) *Manager {
	return &Manager{store: store, base: base, entries: map[string]*entry{}}
}
func (m *Manager) Status() state.ProxyEngine {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, v := range m.entries {
		if v.engine != nil {
			select {
			case <-v.engine.done:
			default:
				n++
			}
		}
	}
	return state.ProxyEngine{Available: Available(), Version: Version, Platform: runtime.GOOS + "/" + runtime.GOARCH, Running: n}
}
func (m *Manager) RoundTrip(req *http.Request) (*http.Response, error) {
	r, ok := req.Context().Value(routeKey{}).(route)
	if !ok || r.provider == "" {
		return m.base.RoundTrip(req)
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrUnavailable
	}
	p, e := m.store.ProxySecretFor(r.provider, r.key)
	if e != nil {
		m.mu.Unlock()
		return nil, ErrUnavailable
	}
	if p == nil {
		m.mu.Unlock()
		return m.base.RoundTrip(req)
	}
	k := p.Profile.ID + ":" + strconv.FormatInt(p.Profile.Revision, 10)
	item := m.entries[k]
	if item != nil && item.engine != nil && item.refs == 0 {
		select {
		case <-item.engine.done:
			m.remove(k, item)
			item = nil
		default:
		}
	}
	if item == nil {
		// Bound retained generations too; a slow stream must not cause unbounded children.
		if len(m.entries) >= 64 {
			m.mu.Unlock()
			return nil, ErrUnavailable
		}
		item = &entry{transport: m.base.Clone()}
		proxyURL := p.Secret
		if p.Profile.Kind == "mihomo" {
			n := 0
			for _, v := range m.entries {
				if v.engine != nil {
					n++
				}
			}
			if n >= 16 {
				m.mu.Unlock()
				return nil, ErrUnavailable
			}
			item.engine, proxyURL, e = startEngine(req.Context(), m.store.Dir, p.Secret)
			if e != nil {
				m.mu.Unlock()
				return nil, ErrUnavailable
			}
		}
		u, err := url.Parse(proxyURL)
		if err != nil {
			if item.engine != nil {
				item.engine.close()
			}
			m.mu.Unlock()
			return nil, ErrUnavailable
		}
		item.transport.Proxy = http.ProxyURL(u)
		m.entries[k] = item
	}
	item.refs++
	m.mu.Unlock()
	resp, e := item.transport.RoundTrip(req)
	if e != nil {
		m.release(k, item)
		return nil, ErrUnavailable
	}
	resp.Body = &responseBody{ReadCloser: resp.Body, release: func() { m.release(k, item) }}
	return resp, nil
}

type responseBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *responseBody) Close() error { e := b.ReadCloser.Close(); b.once.Do(b.release); return e }
func (m *Manager) release(k string, item *entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item.refs--
	if item.retired && item.refs == 0 {
		m.remove(k, item)
	}
}
func (m *Manager) remove(k string, item *entry) {
	item.transport.CloseIdleConnections()
	if item.engine != nil {
		item.engine.close()
	}
	delete(m.entries, k)
}

// Retire only changed profiles. Existing response bodies keep their generation alive.
func (m *Manager) Reconcile(profiles []state.ProxyProfile) {
	live := map[string]bool{}
	for _, p := range profiles {
		live[p.ID+":"+strconv.FormatInt(p.Revision, 10)] = true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.entries {
		if !live[k] {
			v.retired = true
			if v.refs == 0 {
				m.remove(k, v)
			}
		}
	}
}
func (m *Manager) CloseIdleConnections() {
	m.base.CloseIdleConnections()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.entries {
		v.transport.CloseIdleConnections()
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for k, v := range m.entries {
		m.remove(k, v)
	}
	m.base.CloseIdleConnections()
}
