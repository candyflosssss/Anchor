package core

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestFirstOutputTimerStopsAndKeepsTotalDeadline(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	ctx, first := withFirstOutputTimeout(parent, 20*time.Millisecond)
	defer first.close()
	first.observe()
	select {
	case <-ctx.Done():
		t.Fatal("first output did not stop timer")
	case <-time.After(40 * time.Millisecond):
	}
	cancel()
	<-ctx.Done()
	if context.Cause(ctx) != context.Canceled {
		t.Fatal(context.Cause(ctx))
	}
}

func TestProviderTwentyConcurrentRequestsAreDispatchable(t *testing.T) {
	f := setup(t)
	entered := make(chan struct{}, 20)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","status":"completed","output":[]}`)
	}))
	defer up.Close()
	// Ensure the barrier is always released before closing the synthetic server.
	defer once.Do(func() { close(release) })
	f.add(t, "p", up.URL, "synthetic")
	p, _ := f.store.Provider("p")
	p.MaxConcurrent = state.DefaultMaxConcurrent
	if _, err := f.store.PutProvider(p, p.Revision, nil); err != nil {
		t.Fatal(err)
	}
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	results := make(chan int, 20)
	for range 20 {
		go func() {
			req, _ := http.NewRequestWithContext(t.Context(), "POST", f.gateway.URL+"/v1/responses", strings.NewReader(`{"model":"public","input":"test"}`))
			req.Header.Set("Authorization", "Bearer "+f.token)
			resp, err := client.Do(req)
			if err != nil {
				results <- 0
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			results <- resp.StatusCode
		}()
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for range 20 {
		select {
		case <-entered:
		case status := <-results:
			t.Fatalf("request rejected before all 20 dispatched: %d", status)
		case <-deadline.C:
			t.Fatal("20 requests could not dispatch together")
		}
	}
	once.Do(func() { close(release) })
	for range 20 {
		if status := <-results; status != 200 {
			t.Fatal(status)
		}
	}
}

func TestProbeSharesFirstOutputTimeout(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	p, _ := f.store.Provider("p")
	p.FirstTokenTimeoutSeconds = 1
	p, err := f.store.PutProvider(p, p.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.core.inferenceObserved(t.Context(), p, "default", "actual", "", nil, nil, nil)
	if err != nil || result.Failure != "first_token_timeout" || result.HTTPStatus != 200 {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestGatewayFirstOutputTimeout(t *testing.T) {
	for _, mode := range []string{"headers", "body", "heartbeat", "reasoning", "tool", "total"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t)
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				io.Copy(io.Discard, r.Body)
				if mode == "headers" {
					select {
					case <-r.Context().Done():
					case <-time.After(3 * time.Second):
					}
					return
				}
				stream := mode != "body"
				if !stream {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
					select {
					case <-r.Context().Done():
					case <-time.After(3 * time.Second):
					}
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n")
				if mode == "reasoning" || mode == "total" {
					io.WriteString(w, "data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"thinking\"}\n\n")
				}
				if mode == "tool" {
					io.WriteString(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"{}\"}\n\n")
				}
				w.(http.Flusher).Flush()
				ticker := time.NewTicker(50 * time.Millisecond)
				defer ticker.Stop()
				end := time.NewTimer(1300 * time.Millisecond)
				defer end.Stop()
				for {
					select {
					case <-r.Context().Done():
						return
					case <-ticker.C:
						io.WriteString(w, ": heartbeat\n\n")
						w.(http.Flusher).Flush()
					case <-end.C:
						io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
						return
					}
				}
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "synthetic")
			p, _ := f.store.Provider("p")
			p.FirstTokenTimeoutSeconds = 1
			p.TimeoutSeconds = 5
			if mode == "total" {
				p.FirstTokenTimeoutSeconds = 3
				p.TimeoutSeconds = 1
			}
			if _, err := f.store.PutProvider(p, p.Revision, nil); err != nil {
				t.Fatal(err)
			}
			f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
			stream := "true"
			if mode == "body" {
				stream = "false"
			}
			resp := localizedRequest(t, "POST", f.gateway.URL+"/v1/responses", f.token, "zh-CN", `{"model":"public","input":"test","stream":`+stream+`}`)
			raw := text(t, resp)
			if err := f.core.logs.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
			if err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 || row.AttemptCount != 1 {
				t.Fatalf("replayed request: %+v", row)
			}
			switch mode {
			case "reasoning", "tool":
				if row.Status != "succeeded" || !strings.Contains(raw, "response.completed") {
					t.Fatalf("output did not stop timeout: %+v %s", row, raw)
				}
			case "total":
				if row.Status == "succeeded" || row.Code == "upstream_first_token_timeout" {
					t.Fatalf("total deadline lost: %+v", row)
				}
			default:
				if row.Code != "upstream_first_token_timeout" || row.Evidence["first_token_timeout_seconds"] != "1" || strings.Count(raw, "upstream_first_token_timeout") != 1 || strings.Contains(raw, "response.completed") {
					t.Fatalf("%+v %s", row, raw)
				}
				if mode == "headers" && (resp.StatusCode != 504 || row.HTTP != nil) {
					t.Fatalf("%d %+v", resp.StatusCode, row)
				}
				if mode != "headers" && (row.HTTP == nil || *row.HTTP != 200) {
					t.Fatalf("actual HTTP overwritten: %+v", row)
				}
			}
		})
	}
}
