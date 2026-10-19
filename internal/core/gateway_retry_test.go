package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

type channelRetryTestTransport func(*http.Request) (*http.Response, error)

func (f channelRetryTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSingleChannelAttemptBeforeCandidateFallback(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprint(fallback), func(t *testing.T) {
			f := setup(t)
			var bad, good atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				bad.Add(1)
				w.WriteHeader(503)
			}))
			defer up.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if bad.Load() != 1 {
					t.Error("switched candidate before finishing the channel attempt")
				}
				good.Add(1)
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"id":"r","output":[]}`)
			}))
			defer backup.Close()
			f.add(t, "bad", up.URL, "synthetic")
			candidates := []state.Candidate{{ProviderID: "bad", Model: "up"}}
			if fallback {
				f.add(t, "good", backup.URL, "synthetic")
				candidates = append(candidates, state.Candidate{ProviderID: "good", Model: "up"})
			}
			f.model(t, candidates...)
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
			body := text(t, resp)
			wantStatus, wantAttempts := 503, 1
			if fallback {
				wantStatus, wantAttempts = 200, 2
			}
			if resp.StatusCode != wantStatus || bad.Load() != 1 || good.Load() != int32(wantAttempts-1) {
				t.Fatal(resp.StatusCode, bad.Load(), good.Load(), body)
			}
			f.core.logs.Flush(t.Context())
			row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
			if err != nil || row.AttemptCount != wantAttempts {
				t.Fatal(row, err)
			}
			p, _ := f.store.Provider("bad")
			health := f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true})
			if health.Failures != 1 || health.State != "ready" {
				t.Fatal("failed attempts did not update circuit breaker", health)
			}
		})
	}
}

func TestSingleChannelAttemptBeforeSwitchingKeys(t *testing.T) {
	f := setup(t)
	var bad, good atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer synthetic-bad" {
			bad.Add(1)
			w.WriteHeader(502)
			return
		}
		if bad.Load() != 1 {
			t.Error("switched key before finishing the channel attempt")
		}
		good.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","output":[]}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic-bad")
	if err := f.store.MutateProvider("p", 1, "key-put", "second", "synthetic-good", "", true, false); err != nil {
		t.Fatal(err)
	}
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
	body := text(t, resp)
	if resp.StatusCode != 200 || bad.Load() != 1 || good.Load() != 1 {
		t.Fatal(resp.StatusCode, bad.Load(), good.Load(), body)
	}
}

func TestGatewayFailoverSkipAuthenticationAndRateLimits(t *testing.T) {
	for _, status := range []int{401, 403, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := setup(t)
			var bad, good atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				bad.Add(1)
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(status)
			}))
			defer up.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				good.Add(1)
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"id":"r","output":[]}`)
			}))
			defer backup.Close()
			f.add(t, "bad", up.URL, "synthetic")
			f.add(t, "good", backup.URL, "synthetic")
			f.model(t, state.Candidate{ProviderID: "bad", Model: "up"}, state.Candidate{ProviderID: "good", Model: "up"})
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
			body := text(t, resp)
			if resp.StatusCode != 200 || bad.Load() != 1 || good.Load() != 1 {
				t.Fatal(resp.StatusCode, bad.Load(), good.Load(), body)
			}
		})
	}
}

func TestGatewayFailoverConnectionFailuresOnlyBeforeGotConn(t *testing.T) {
	for _, connected := range []bool{false, true} {
		t.Run(fmt.Sprint(connected), func(t *testing.T) {
			f := setup(t)
			var calls atomic.Int32
			f.add(t, "p", "https://upstream.invalid/v1", "synthetic")
			f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
			f.core.upstream.Transport = channelRetryTestTransport(func(r *http.Request) (*http.Response, error) {
				n := calls.Add(1)
				if connected {
					httptrace.ContextClientTrace(r.Context()).GotConn(httptrace.GotConnInfo{})
				}
				if n < 4 {
					return nil, errors.New("synthetic connection failure")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"r","output":[]}`))}, nil
			})
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
			body := text(t, resp)
			wantStatus, wantCalls := 503, int32(1)
			if connected {
				wantStatus, wantCalls = 502, 1
			}
			if resp.StatusCode != wantStatus || calls.Load() != wantCalls {
				t.Fatal(resp.StatusCode, calls.Load(), body)
			}
		})
	}
}

func TestGatewayFailoverDoNotReplayCommittedStream(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","stream":true}`)
	body := text(t, resp)
	if resp.StatusCode != 200 || calls.Load() != 1 || !strings.Contains(body, "partial") {
		t.Fatal(resp.StatusCode, calls.Load(), body)
	}
	f.core.logs.Flush(t.Context())
	row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
	if err != nil || row.Status != "failed" || row.Code != "stream_interrupted" || row.AttemptCount != 1 {
		t.Fatal(row, err)
	}
}

func TestGatewayFailoverStopWhenClientCancels(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	seen := make(chan struct{})
	f.add(t, "p", "https://upstream.invalid/v1", "synthetic")
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	f.core.upstream.Transport = channelRetryTestTransport(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(seen)
		}
		return &http.Response{StatusCode: 503, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := httptest.NewRequest("POST", f.gateway.URL+"/v1/responses", strings.NewReader(`{"model":"public"}`)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		f.core.Gateway().ServeHTTP(w, r)
		close(done)
	}()
	select {
	case <-seen:
	case <-time.After(2 * time.Second):
		t.Fatal("first attempt did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("retry backoff ignored client cancellation")
	}
	if calls.Load() != 1 {
		t.Fatal("retried a cancelled request", calls.Load())
	}
	p, _ := f.store.Provider("p")
	if health := f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true}); health.Failures != 0 || health.State != "ready" {
		t.Fatal("client cancellation damaged channel health", health)
	}
}

func TestGatewayFailoverDoNotReplayRejectedOrInvalidResponses(t *testing.T) {
	for _, status := range []int{400, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := setup(t)
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(status)
				io.WriteString(w, "invalid inference response")
			}))
			defer up.Close()
			f.add(t, "p", up.URL+"/v1", "synthetic")
			f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
			body := text(t, resp)
			if resp.StatusCode != 502 || calls.Load() != 1 {
				t.Fatal(resp.StatusCode, calls.Load(), body)
			}
		})
	}
}
