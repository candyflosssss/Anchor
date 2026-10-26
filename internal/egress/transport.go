package egress

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/candyflosssss/Anchor/internal/systemproxy"
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
	profile   state.ProxyProfile
}
type Manager struct {
	store   *state.Store
	base    *http.Transport
	system  *http.Transport
	mu      sync.Mutex
	entries map[string]*entry
	closed  bool
}

func New(store *state.Store, base *http.Transport) *Manager {
	// Direct is an explicit application-layer policy, independent of ambient
	// HTTP_PROXY. It cannot override an OS TUN or a router's transparent proxy.
	base = base.Clone()
	base.Proxy = nil
	base.DialContext = localhostDialer(base.DialContext)
	system := base.Clone()
	system.Proxy = requiredSystemProxy
	return &Manager{store: store, base: base, system: system, entries: map[string]*entry{}}
}

// Pin locally dialed localhost to loopback without DNS or hosts-file lookups.
// The URL and Host header stay intact, and explicit proxy routing is preserved.
func localhostDialer(dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || !strings.EqualFold(host, "localhost") {
			return dial(ctx, network, address)
		}
		if network == "tcp6" {
			return dial(ctx, network, net.JoinHostPort("::1", port))
		}
		conn, err := dial(ctx, network, net.JoinHostPort("127.0.0.1", port))
		if err == nil || network == "tcp4" {
			return conn, err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return dial(ctx, network, net.JoinHostPort("::1", port))
	}
}
func (m *Manager) Status() state.ProxyEngine {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	listeners := []state.ProxyListener{}
	for _, v := range m.entries {
		if v.engine != nil {
			select {
			case <-v.engine.done:
			default:
				n++
				listeners = append(listeners, state.ProxyListener{ProfileID: v.profile.ID, Revision: v.profile.Revision, Port: v.engine.port, Retiring: v.retired})
			}
		}
	}
	sort.Slice(listeners, func(i, j int) bool {
		if listeners[i].ProfileID == listeners[j].ProfileID {
			return listeners[i].Revision < listeners[j].Revision
		}
		return listeners[i].ProfileID < listeners[j].ProfileID
	})
	return state.ProxyEngine{Available: Available(), Version: Version, Platform: runtime.GOOS + "/" + runtime.GOARCH, Running: n, Listeners: listeners}
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
	if p.Mode == "system" {
		m.mu.Unlock()
		resp, err := m.system.RoundTrip(req)
		if err != nil {
			return nil, ErrUnavailable
		}
		return resp, nil
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
		item = &entry{transport: m.base.Clone(), profile: p.Profile}
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
			item.engine, proxyURL, e = startEngine(req.Context(), m.store.Dir, p.Secret, *p.Profile.ListenPort)
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
	m.system.CloseIdleConnections()
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
	m.base.CloseIdleConnections()
	m.system.CloseIdleConnections()
	for k, v := range m.entries {
		m.remove(k, v)
	}
}

// A selected proxy route must never silently become direct: absent settings,
// NO_PROXY, OS bypass rules and PAC DIRECT are failures for supplier requests.
func requiredSystemProxy(req *http.Request) (*url.URL, error) {
	proxy, err := systemproxy.ForRequest(req)
	if err != nil || proxy == nil {
		return nil, ErrUnavailable
	}
	return proxy, nil
}
