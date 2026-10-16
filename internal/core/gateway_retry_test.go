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

func TestSameChannelRetriesRecoverOnFourthAttempt(t *testing.T) {
	for _, status := range []int{502, 503, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := setup(t)
			var calls atomic.Int32
			var firstBody string
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if calls.Add(1) == 1 {
					firstBody = string(body)
				} else if string(body) != firstBody {
					t.Error("retry changed the upstream request")
				}
				if r.Header.Get("Authorization") != "Bearer synthetic" {
					t.Error("retry lost the selected key")
				}
				if calls.Load() < 4 {
					w.WriteHeader(status)
					io.WriteString(w, "private upstream detail")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"id":"r","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":2}}`)
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "synthetic")
			f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
			body := text(t, resp)
			if resp.StatusCode != 200 || calls.Load() != 4 || strings.Contains(body, "private") {
				t.Fatal(resp.StatusCode, calls.Load(), body)
			}
			if err := f.core.logs.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
			if err != nil || row.Status != "succeeded" || row.AttemptCount != 4 || len(row.Attempts) != 4 {
				t.Fatal(row, err)
			}
			for i, attempt := range row.Attempts {
				if attempt.Number != i+1 || attempt.Provider != "p" || attempt.Key != "default" {
					t.Fatal(attempt)
				}
				if i < 3 && (attempt.Outcome != "retry" || attempt.HTTP == nil || *attempt.HTTP != status) {
					t.Fatal(attempt)
				}
			}
			usage, err := f.store.Usage("p")
			if err != nil || len(usage) != 1 || usage[0].Requests != 4 || usage[0].Failures != 3 || usage[0].Known != 1 {
				t.Fatal(usage, err)
			}
			p, _ := f.store.Provider("p")
			if health := f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true}); health.Failures != 0 || health.State != "ready" {
				t.Fatal(health)
			}
		})
	}
}

func TestSameChannelRetriesExhaustBeforeCandidateFallback(t *testing.T) {
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
				if bad.Load() != 4 {
					t.Error("switched candidate before exhausting same-channel retries")
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
			wantStatus, wantAttempts := 503, 4
			if fallback {
				wantStatus, wantAttempts = 200, 5
			}
			if resp.StatusCode != wantStatus || bad.Load() != 4 || good.Load() != int32(wantAttempts-4) {
				t.Fatal(resp.StatusCode, bad.Load(), good.Load(), body)
			}
			f.core.logs.Flush(t.Context())
			row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
			if err != nil || row.AttemptCount != wantAttempts {
				t.Fatal(row, err)
			}
			p, _ := f.store.Provider("bad")
			health := f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true})
			if health.Failures != 4 || health.State != "cooling" {
				t.Fatal("failed attempts did not update circuit breaker", health)
			}
		})
	}
}

func TestSameChannelRetriesBeforeSwitchingKeys(t *testing.T) {
	f := setup(t)
	var bad, good atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer synthetic-bad" {
			bad.Add(1)
			w.WriteHeader(502)
			return
		}
		if bad.Load() != 4 {
			t.Error("switched key before exhausting same-channel retries")
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
	if resp.StatusCode != 200 || bad.Load() != 4 || good.Load() != 1 {
		t.Fatal(resp.StatusCode, bad.Load(), good.Load(), body)
	}
}

func TestSameChannelRetriesSkipAuthenticationAndRateLimits(t *testing.T) {
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

func TestSameChannelRetriesConnectionFailuresOnlyBeforeGotConn(t *testing.T) {
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
			wantStatus, wantCalls := 200, int32(4)
			if connected {
				wantStatus, wantCalls = 502, 1
			}
			if resp.StatusCode != wantStatus || calls.Load() != wantCalls {
				t.Fatal(resp.StatusCode, calls.Load(), body)
			}
		})
	}
}

func TestSameChannelRetriesDoNotReplayCommittedStream(t *testing.T) {
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

func TestSameChannelRetriesShareProviderTimeout(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	p, _ := f.store.Provider("p")
	p.TimeoutSeconds = 1
	if _, err := f.store.PutProvider(p, p.Revision, nil); err != nil {
		t.Fatal(err)
	}
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	started := time.Now()
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
	body := text(t, resp)
	if resp.StatusCode != 504 || !strings.Contains(body, "upstream_timeout") || calls.Load() < 1 || calls.Load() >= 4 || time.Since(started) > 2*time.Second {
		t.Fatal(resp.StatusCode, calls.Load(), time.Since(started), body)
	}
}

func TestChannelRetryWaitHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := waitChannelRetry(ctx, 2); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSameChannelRetriesKeepHalfOpenReservation(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	var provider state.Provider
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if health := f.core.keyStatus(provider, state.ProviderKey{ID: "default", Enabled: true}); health.State != "half-open" {
			t.Error("half-open reservation released during retry group", health)
		}
		if calls.Add(1) < 4 {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","output":[]}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	provider, _ = f.store.Provider("p")
	f.core.mu.Lock()
	f.core.health[healthID(provider, "default")] = &keyHealth{Failures: 3, Until: time.Now().Add(-time.Second)}
	f.core.mu.Unlock()
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
	body := text(t, resp)
	if resp.StatusCode != 200 || calls.Load() != 4 {
		t.Fatal(resp.StatusCode, calls.Load(), body)
	}
}

func TestSameChannelRetriesStopWhenClientCancels(t *testing.T) {
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

func TestSameChannelRetriesDoNotReplayRejectedOrInvalidResponses(t *testing.T) {
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
