package egress

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestNormalize(t *testing.T) {
	for _, raw := range []string{"http://user:pass@localhost:8080", "https://localhost:443"} {
		if _, e := Normalize("http", raw, ""); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := Normalize("socks5", "socks5://localhost:1080", ""); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{"file:///etc/passwd", "http://localhost:0", "http://127.0.0.1:1/path", "http://127.0.0.1:1?secret=x", "socks5://localhost:1"} {
		if _, e := Normalize("http", raw, ""); e == nil {
			t.Fatal(raw)
		}
	}
	valid := "proxies:\n - {name: first, type: ss, server: 127.0.0.1, port: 1234, cipher: aes-128-gcm, password: synthetic}\n - {name: second, type: http, server: localhost, port: 1235}\nexternal-controller: 0.0.0.0:9090\ntun: {enable: true}"
	raw, e := Normalize("mihomo", valid, "second")
	if e != nil || strings.Contains(raw, "tun") || strings.Contains(raw, "first") || strings.Contains(raw, "9090") {
		t.Fatal(raw, e)
	}
	if _, e = Normalize("mihomo", valid, ""); e == nil {
		t.Fatal("ambiguous nodes")
	}
	for _, raw := range []string{"{name: x, type: ss, server: localhost, port: 1, dialer-proxy: other}", "{name: x, type: http, server: localhost, port: 1, tls: true, certificate: /etc/passwd}", "{name: &x foo, type: http, server: *x, port: 1}"} {
		if _, e = Normalize("mihomo", raw, ""); e == nil {
			t.Fatal("unsafe", raw)
		}
	}
}
func testManager(t *testing.T) (*Manager, *state.Store) {
	t.Helper()
	s, e := state.Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	secret := "synthetic"
	_, e = s.PutProvider(state.Provider{ID: "p", BaseURL: "http://127.0.0.1:1", Protocol: "responses", Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 2}, 0, &secret)
	if e != nil {
		t.Fatal(e)
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	m := New(s, tr)
	t.Cleanup(m.Close)
	return m, s
}
func mutate(t *testing.T, s *state.Store, in state.ProxyAction) {
	t.Helper()
	w, e := s.ProxyWorkspace()
	if e != nil {
		t.Fatal(e)
	}
	in.ExpectedRevision = w.Revision
	in.Approved = true
	if e = s.MutateProxy(in, Normalize); e != nil {
		t.Fatal(e)
	}
}
func put(t *testing.T, s *state.Store, kind, raw string) {
	t.Helper()
	mutate(t, s, state.ProxyAction{Action: "put", Profile: state.ProxyProfile{ID: "edge", Name: "Edge", Kind: kind}, Secret: &raw})
}
func bind(t *testing.T, s *state.Store, key, mode string) {
	t.Helper()
	id := ""
	if mode == "proxy" {
		id = "edge"
	}
	mutate(t, s, state.ProxyAction{Action: "bind", Binding: state.ProxyBinding{ProviderID: "p", KeyID: key, Mode: mode, ProxyID: id}})
}
func get(t *testing.T, m *Manager, target string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(WithRoute(context.Background(), "p", "default"), 10*time.Second)
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", target, nil)
	resp, e := (&http.Client{Transport: m}).Do(req)
	if e != nil {
		t.Fatal(e)
	}
	return resp
}
func TestHTTPProxyDirectOverrideAndDrain(t *testing.T) {
	m, s := testManager(t)
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "direct") }))
	defer direct.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.IsAbs() {
			t.Error("proxy absolute form missing")
		}
		io.WriteString(w, "proxy")
	}))
	defer proxy.Close()
	put(t, s, "http", proxy.URL)
	bind(t, s, "", "proxy")
	resp := get(t, m, direct.URL)
	data, _ := io.ReadAll(resp.Body)
	if string(data) != "proxy" {
		t.Fatal(string(data))
	}
	old := m.entries["edge:1"]
	put(t, s, "http", proxy.URL)
	w, _ := s.ProxyWorkspace()
	m.Reconcile(w.Profiles)
	if !old.retired || m.entries["edge:1"] == nil {
		t.Fatal("active body was retired too early")
	}
	resp.Body.Close()
	if m.entries["edge:1"] != nil {
		t.Fatal("old pool leaked")
	}
	bind(t, s, "default", "direct")
	resp = get(t, m, direct.URL)
	data, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "direct" {
		t.Fatal(string(data))
	}
	bind(t, s, "default", "inherit")
	proxy.Close()
	req, _ := http.NewRequestWithContext(WithRoute(context.Background(), "p", "default"), "GET", direct.URL, nil)
	if _, e := m.RoundTrip(req); !errors.Is(e, ErrUnavailable) {
		t.Fatal("must not fall back", e)
	}
}
func TestBundledMihomoLocalNodeLifecycle(t *testing.T) {
	if !Available() {
		t.Skip("pinned payload not prepared")
	}
	m, s := testManager(t)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "via-mihomo") }))
	defer proxy.Close()
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(proxy.URL, "http://"))
	raw := fmt.Sprintf("name: local\ntype: http\nserver: %s\nport: %s\n", host, port)
	put(t, s, "mihomo", raw)
	bind(t, s, "", "proxy")
	resp := get(t, m, "http://example.invalid/test")
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "via-mihomo" || m.Status().Running != 1 {
		t.Fatal(string(data), m.Status())
	}
	entry := m.entries["edge:1"]
	probe, _ := http.NewRequest(http.MethodGet, "http://example.invalid/auth-check", nil)
	endpoint, err := entry.transport.Proxy(probe)
	if err != nil {
		t.Fatal(err)
	}
	endpoint.User = nil
	unauthenticated := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(endpoint)}, Timeout: 2 * time.Second}
	defer unauthenticated.CloseIdleConnections()
	denied, err := unauthenticated.Do(probe)
	if err != nil {
		t.Fatal(err)
	}
	denied.Body.Close()
	if denied.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("loopback proxy accepted unauthenticated request: %d", denied.StatusCode)
	}
	entry.engine.close()
	resp = get(t, m, "http://example.invalid/restart")
	data, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "via-mihomo" || m.entries["edge:1"] == entry {
		t.Fatal("dead engine was not replaced")
	}
	entry = m.entries["edge:1"]
	m.Close()
	select {
	case <-entry.engine.done:
	case <-time.After(3 * time.Second):
		t.Fatal("orphan child")
	}
}
func TestPublicWorkspaceDoesNotExposeNode(t *testing.T) {
	_, s := testManager(t)
	put(t, s, "mihomo", "{type: http, name: test, server: localhost, port: 1234, password: private-node-password}")
	w, _ := s.ProxyWorkspace()
	data, _ := json.Marshal(w)
	if strings.Contains(string(data), "private-node-password") || strings.Contains(string(data), "localhost") {
		t.Fatal(string(data))
	}
}

func TestSOCKS5UsesRemoteHostname(t *testing.T) {
	m, s := testManager(t)
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		c, e := listener.Accept()
		if e != nil {
			done <- e
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		head := make([]byte, 2)
		if _, e = io.ReadFull(c, head); e != nil {
			done <- e
			return
		}
		methods := make([]byte, int(head[1]))
		_, e = io.ReadFull(c, methods)
		if e != nil {
			done <- e
			return
		}
		_, _ = c.Write([]byte{5, 0})
		request := make([]byte, 5)
		if _, e = io.ReadFull(c, request); e != nil {
			done <- e
			return
		}
		if request[3] != 3 {
			done <- fmt.Errorf("expected remote hostname, got %d", request[3])
			return
		}
		host := make([]byte, int(request[4])+2)
		if _, e = io.ReadFull(c, host); e != nil {
			done <- e
			return
		}
		if string(host[:len(host)-2]) != "example.invalid" {
			done <- fmt.Errorf("wrong host")
			return
		}
		_, _ = c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 80})
		reader := bufio.NewReader(c)
		req, e := http.ReadRequest(reader)
		if e != nil {
			done <- e
			return
		}
		req.Body.Close()
		_, e = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 5\r\nConnection: close\r\n\r\nsocks")
		done <- e
	}()
	put(t, s, "socks5", "socks5h://"+listener.Addr().String())
	bind(t, s, "", "proxy")
	resp := get(t, m, "http://example.invalid/test")
	b, e := io.ReadAll(resp.Body)
	resp.Body.Close()
	if e != nil || string(b) != "socks" {
		t.Fatal(string(b), e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
}
func TestHTTPSConnectProxy(t *testing.T) {
	m, s := testManager(t)
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tls") }))
	defer origin.Close()
	m.base.TLSClientConfig = origin.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Host != strings.TrimPrefix(origin.URL, "https://") {
			t.Error("unexpected CONNECT")
			w.WriteHeader(400)
			return
		}
		upstream, e := net.Dial("tcp", r.Host)
		if e != nil {
			w.WriteHeader(502)
			return
		}
		defer upstream.Close()
		conn, _, e := w.(http.Hijacker).Hijack()
		if e != nil {
			return
		}
		defer conn.Close()
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go func() { _, _ = io.Copy(upstream, conn) }()
		_, _ = io.Copy(conn, upstream)
	}))
	defer proxy.Close()
	put(t, s, "http", proxy.URL)
	bind(t, s, "", "proxy")
	resp := get(t, m, origin.URL)
	b, e := io.ReadAll(resp.Body)
	resp.Body.Close()
	if e != nil || string(b) != "tls" {
		t.Fatal(string(b), e)
	}
}
