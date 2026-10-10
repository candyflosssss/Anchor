package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

type fixture struct {
	store            *state.Store
	core             *Server
	gateway, control *httptest.Server
	owner, token     string
	logs             *bytes.Buffer
}

func setup(t *testing.T) *fixture {
	t.Helper()
	st, e := state.Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	if _, e = st.Initialize("127.0.0.1:17861", "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	owner, e := st.OwnerToken()
	if e != nil {
		t.Fatal(e)
	}
	_, token, e := st.Grant("test", "gateway", 0)
	if e != nil {
		t.Fatal(e)
	}
	logs := new(bytes.Buffer)
	srv := New(st, slog.New(slog.NewJSONHandler(logs, nil)))
	t.Cleanup(srv.Close)
	control := httptest.NewServer(srv.Control())
	t.Cleanup(control.Close)
	gateway := httptest.NewServer(srv.Gateway())
	t.Cleanup(gateway.Close)
	return &fixture{st, srv, gateway, control, owner, token, logs}
}
func (f *fixture) add(t *testing.T, id, base, secret string) {
	t.Helper()
	_, e := f.store.PutProvider(state.Provider{ID: id, BaseURL: base, Protocol: "responses", Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, 0, &secret)
	if e != nil {
		t.Fatal(e)
	}
}
func (f *fixture) model(t *testing.T, candidates ...state.Candidate) {
	t.Helper()
	_, e := f.store.PutModel(state.Model{ID: "public", Tools: true, Streaming: true, Candidates: candidates}, 0)
	if e != nil {
		t.Fatal(e)
	}
}
func request(t *testing.T, method, url, token, body string) *http.Response {
	t.Helper()
	r, e := http.NewRequest(method, url, strings.NewReader(body))
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	cl := &http.Client{Timeout: 5 * time.Second}
	resp, e := cl.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { resp.Body.Close(); cl.CloseIdleConnections() })
	return resp
}
func text(t *testing.T, r *http.Response) string {
	t.Helper()
	b, e := io.ReadAll(r.Body)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func TestAuthenticationAndHostBoundaries(t *testing.T) {
	f := setup(t)
	for _, tc := range []struct {
		url, token string
		status     int
	}{{f.control.URL + "/control/v1/status", "", 401}, {f.control.URL + "/control/v1/status", f.token, 401}, {f.gateway.URL + "/v1/models", f.owner, 401}, {f.gateway.URL + "/v1/models", f.token, 200}, {f.control.URL + "/control/v1/status", f.owner, 200}} {
		r := request(t, "GET", tc.url, tc.token, "")
		if r.StatusCode != tc.status {
			t.Fatal(tc.url, r.StatusCode, text(t, r))
		}
	}
	for _, kind := range []string{"origin", "host"} {
		r, _ := http.NewRequest("GET", f.control.URL+"/control/v1/status", nil)
		r.Header.Set("Authorization", "Bearer "+f.owner)
		if kind == "origin" {
			r.Header.Set("Origin", "https://evil.invalid")
		} else {
			r.Host = "evil.invalid:80"
		}
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 && resp.StatusCode != 400 {
			t.Fatal(kind, resp.StatusCode)
		}
	}
}
func TestManagementRevisionIdentityAndRedaction(t *testing.T) {
	f := setup(t)
	cfg, _ := f.store.Settings()
	secret := "SECRET-DO-NOT-OUTPUT"
	body := `{"provider":{"id":"p","base_url":"https://example.invalid/v1","protocol":"responses","enabled":true,"timeout_seconds":5,"max_concurrent":1},"expected_revision":0,"secret":"` + secret + `"}`
	put := func(identity string) *http.Response {
		r, _ := http.NewRequest("PUT", f.control.URL+"/control/v1/providers/p", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+f.owner)
		r.Header.Set("X-Anchor-Instance", identity)
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		return resp
	}
	wrong := put("other")
	if wrong.StatusCode != 409 {
		t.Fatal(wrong.StatusCode)
	}
	wrong.Body.Close()
	resp := put(cfg.InstanceID)
	b := text(t, resp)
	resp.Body.Close()
	if resp.StatusCode != 200 || strings.Contains(b, secret) {
		t.Fatal(resp.StatusCode, b)
	}
	duplicate := put(cfg.InstanceID)
	if duplicate.StatusCode != 409 {
		t.Fatal(duplicate.StatusCode)
	}
	duplicate.Body.Close()
	listed := request(t, "GET", f.control.URL+"/control/v1/providers", f.owner, "")
	if b = text(t, listed); strings.Contains(b, secret) || !strings.Contains(b, `"has_secret":true`) {
		t.Fatal(b)
	}
}
func TestResponsesMappingAndToolResultPassthrough(t *testing.T) {
	for _, providerID := range []string{"p", "摸鱼AI"} {
		t.Run(providerID, func(t *testing.T) {
			f := setup(t)
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer provider-secret" {
					t.Errorf("bad upstream path/auth: %s", r.URL.Path)
				}
				if r.Header.Get("X-Anchor-Instance") != "" {
					t.Error("management metadata leaked")
				}
				var in map[string]json.RawMessage
				if e := json.NewDecoder(r.Body).Decode(&in); e != nil {
					t.Error(e)
				}
				if string(in["model"]) != `"actual-model"` || string(in["store"]) != "false" {
					t.Error("mapping/storage policy failed")
				}
				w.Header().Set("Content-Type", "application/json")
				if bytes.Contains(in["input"], []byte("function_call_output")) {
					io.WriteString(w, `{"id":"r2","output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]}`)
				} else {
					io.WriteString(w, `{"id":"r1","output":[{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"test.txt\"}"}]}`)
				}
			}))
			defer up.Close()
			f.add(t, providerID, up.URL+"/v1", "provider-secret")
			f.model(t, state.Candidate{ProviderID: providerID, Model: "actual-model"})
			first := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hello","tools":[{"type":"function","name":"read_file","parameters":{"type":"object"}}]}`)
			if first.StatusCode != 200 || !strings.Contains(text(t, first), "call_1") {
				t.Fatal("tool call failed")
			}
			second := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":[{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"file contents"}]}`)
			if second.StatusCode != 200 || !strings.Contains(text(t, second), "done") || calls.Load() != 2 {
				t.Fatal("tool result roundtrip failed")
			}
			log := f.logs.String()
			for _, s := range []string{"provider-secret", f.token, "file contents", "hello"} {
				if strings.Contains(log, s) {
					t.Fatal("log leaked secret/body")
				}
			}
			if !strings.Contains(log, "actual-model") {
				t.Fatal("route evidence missing")
			}
		})
	}
}

func TestStreamingIsFlushedAndPreserved(t *testing.T) {
	f := setup(t)
	release := make(chan struct{})
	first := "event: response.function_call_arguments.delta\ndata: {\"delta\":\"{\\\"x\\\":\"}\n\n"
	last := "event: response.completed\ndata: {\"usage\":{\"input_tokens\":1}}\n\n"
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, first)
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		io.WriteString(w, last)
	}))
	defer func() { close(release); up.Close() }()
	f.add(t, "p", up.URL, "")
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"test","stream":true}`)
	b := make([]byte, len(first))
	if _, e := io.ReadFull(resp.Body, b); e != nil {
		t.Fatal(e)
	}
	if string(b) != first {
		t.Fatal("SSE changed")
	}
	release <- struct{}{}
	if tail := text(t, resp); tail != last {
		t.Fatal(tail)
	}
}
func TestFallbackOnlyBeforeDownstreamOutput(t *testing.T) {
	f := setup(t)
	var primary, backup atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primary.Add(1)
		w.WriteHeader(503)
		io.WriteString(w, "secret error body")
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backup.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"output":[]}`)
	}))
	defer b.Close()
	f.add(t, "a", a.URL, "")
	f.add(t, "b", b.URL, "")
	f.model(t, state.Candidate{ProviderID: "a", Model: "one"}, state.Candidate{ProviderID: "b", Model: "two"})
	r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
	if r.StatusCode != 200 || strings.Contains(text(t, r), "secret") || primary.Load() != 1 || backup.Load() != 1 {
		t.Fatal("fallback failed")
	}
}
func TestUnauthorizedUpstreamDoesNotFallbackOrLeak(t *testing.T) {
	f := setup(t)
	var backup atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, "SECRET authentication failure")
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { backup.Add(1) }))
	defer b.Close()
	f.add(t, "a", a.URL, "")
	f.add(t, "b", b.URL, "")
	f.model(t, state.Candidate{ProviderID: "a", Model: "one"}, state.Candidate{ProviderID: "b", Model: "two"})
	r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
	if r.StatusCode != 502 || strings.Contains(text(t, r), "SECRET") || backup.Load() != 0 {
		t.Fatal("unsafe fallback/error reflection")
	}
}
func TestDisconnectCancelsUpstreamAndNeverReplays(t *testing.T) {
	f := setup(t)
	started, canceled := make(chan struct{}), make(chan struct{})
	var backup atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { backup.Add(1) }))
	defer b.Close()
	f.add(t, "a", a.URL, "")
	f.add(t, "b", b.URL, "")
	f.model(t, state.Candidate{ProviderID: "a", Model: "one"}, state.Candidate{ProviderID: "b", Model: "two"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "POST", f.gateway.URL+"/v1/responses", strings.NewReader(`{"model":"public","stream":true}`))
	r.Header.Set("Authorization", "Bearer "+f.token)
	resp, e := http.DefaultClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream did not start")
	}
	cancel()
	resp.Body.Close()
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not propagate")
	}
	if backup.Load() != 0 {
		t.Fatal("stream replayed")
	}
}
func TestCapabilitiesAndUnsupportedStateRejected(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "https://example.invalid/v1", "")
	if _, e := f.store.PutModel(state.Model{ID: "public", Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}, 0); e != nil {
		t.Fatal(e)
	}
	for _, body := range []string{`{"model":"unknown"}`, `{"model":"public","stream":true}`, `{"model":"public","tools":[{"type":"function","name":"f"}]}`, `{"model":"public","previous_response_id":"r1"}`, `{"model":"public","store":true}`, `{"model":"public","conversation":"conv-id"}`, `{"model":"public","background":true}`, `{"model":"public","tools":[{"type":"web_search"}]}`, `{"model":"public","stream":"yes"}`, `null`} {
		r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, body)
		if r.StatusCode != 400 && r.StatusCode != 404 {
			t.Fatal(body, r.StatusCode, text(t, r))
		}
	}
}
func TestOversizeBody(t *testing.T) {
	f := setup(t)
	r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, strings.Repeat("x", MaxRequestBytes+1))
	if r.StatusCode != 413 {
		t.Fatal(r.StatusCode)
	}
}
func TestProviderConcurrencyLimit(t *testing.T) {
	f := setup(t)
	started, release := make(chan struct{}), make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "")
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	done := make(chan int, 1)
	go func() {
		r, _ := http.NewRequest("POST", f.gateway.URL+"/v1/responses", strings.NewReader(`{"model":"public"}`))
		r.Header.Set("Authorization", "Bearer "+f.token)
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			done <- 0
			return
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		done <- resp.StatusCode
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("request did not start")
	}
	second := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
	status := second.StatusCode
	close(release)
	if <-done != 200 || status != 503 {
		t.Fatal("provider concurrency not bounded", status)
	}
}
func TestRedirectIsNotFollowed(t *testing.T) {
	f := setup(t)
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer target.Close()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer up.Close()
	f.add(t, "p", up.URL, "provider-secret")
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
	if r.StatusCode != 502 || leaked.Load() != 0 {
		t.Fatal("redirect followed")
	}
}
func TestListenerFailureReleasesFirstPort(t *testing.T) {
	gateway, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer gateway.Close()
	control, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	address := control.Addr().String()
	control.Close()
	st, e := state.Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	if _, e = st.Initialize(address, gateway.Addr().String()); e != nil {
		t.Fatal(e)
	}
	srv := New(st, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e = srv.Run(ctx); e == nil {
		t.Fatal("occupied gateway port accepted")
	}
	ln, e := net.Listen("tcp", address)
	if e != nil {
		t.Fatal("management port leaked", e)
	}
	ln.Close()
}

func TestPartialStreamFailureNeverReplays(t *testing.T) {
	f := setup(t)
	var backup atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, ok := w.(http.Hijacker)
		if !ok {
			t.Error("missing hijack support")
			return
		}
		conn, b, e := h.Hijack()
		if e != nil {
			t.Error(e)
			return
		}
		defer conn.Close()
		b.WriteString("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\nD\r\ndata: first\n\n\r\n")
		b.Flush()
		// Deliberately omit the terminating chunk, simulating an interrupted upstream.
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { backup.Add(1) }))
	defer b.Close()
	f.add(t, "a", a.URL, "")
	f.add(t, "b", b.URL, "")
	f.model(t, state.Candidate{ProviderID: "a", Model: "one"}, state.Candidate{ProviderID: "b", Model: "two"})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","stream":true}`)
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(body) != "data: first\n\n" || backup.Load() != 0 {
		t.Fatal("partial stream was altered/replayed", resp.StatusCode, string(body))
	}
}
func TestAmbiguousTimeoutDoesNotReplay(t *testing.T) {
	f := setup(t)
	var backup atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); <-r.Context().Done() }))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { backup.Add(1) }))
	defer b.Close()
	f.add(t, "a", a.URL, "")
	f.add(t, "b", b.URL, "")
	p, e := f.store.Provider("a")
	if e != nil {
		t.Fatal(e)
	}
	p.TimeoutSeconds = 1
	if _, e = f.store.PutProvider(p, 1, nil); e != nil {
		t.Fatal(e)
	}
	f.model(t, state.Candidate{ProviderID: "a", Model: "one"}, state.Candidate{ProviderID: "b", Model: "two"})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
	if resp.StatusCode != 504 || backup.Load() != 0 {
		t.Fatal("ambiguous timeout replayed", resp.StatusCode)
	}
}
func TestDisabledCandidateAndUnknownModelDoNotUseDefault(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer up.Close()
	f.add(t, "p", up.URL, "")
	p, e := f.store.Provider("p")
	if e != nil {
		t.Fatal(e)
	}
	p.Enabled = false
	if _, e = f.store.PutProvider(p, 1, nil); e != nil {
		t.Fatal(e)
	}
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	for _, tc := range []struct {
		body   string
		status int
	}{{`{"model":"public"}`, 503}, {`{"model":"unconfigured"}`, 404}} {
		resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, tc.body)
		if resp.StatusCode != tc.status {
			t.Fatal(resp.StatusCode)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("disabled/default provider used")
	}
}
func TestMalformedManagementJSONIsRejected(t *testing.T) {
	f := setup(t)
	cfg, _ := f.store.Settings()
	for _, body := range []string{`{"unknown":true}`, `{} {}`, strings.Repeat("x", 128<<10+1)} {
		r, _ := http.NewRequest("PUT", f.control.URL+"/control/v1/providers/p", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+f.owner)
		r.Header.Set("X-Anchor-Instance", cfg.InstanceID)
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatal(resp.StatusCode)
		}
	}
}

func TestGatewayGlobalConcurrencyLimit(t *testing.T) {
	f := setup(t)
	for i := 0; i < cap(f.core.slots); i++ {
		f.core.slots <- struct{}{}
	}
	defer func() {
		for len(f.core.slots) > 0 {
			<-f.core.slots
		}
	}()
	response := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"unconfigured"}`)
	if response.StatusCode != 429 {
		t.Fatal("global limit was not enforced", response.StatusCode)
	}
}
