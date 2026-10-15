package core

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func logClient(t *testing.T, f *fixture) *client.Client {
	cfg, _ := f.store.Settings()
	c, _, err := client.Connect(t.Context(), state.Context{Name: "test", Endpoint: f.control.URL, InstanceID: cfg.InstanceID, CredentialRef: "local-owner"}, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}
func TestRealLogsQueryDetailExportRetentionAndIdentity(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","status":"completed","output":[],"usage":{"input_tokens":12,"output_tokens":7,"output_tokens_details":{"reasoning_tokens":0}}}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "DO_NOT_STORE")
	f.model(t, state.Candidate{ProviderID: "p", Model: "upstream"})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"DO_NOT_STORE","reasoning":{"effort":"high"}}`)
	io.Copy(io.Discard, resp.Body)
	id := resp.Header.Get("X-Anchor-Request-ID")
	if resp.StatusCode != 200 || id == "" {
		t.Fatal(resp.StatusCode, id)
	}
	if err := f.core.logs.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	c := logClient(t, f)
	p, err := c.Logs(t.Context(), logbook.Query{Model: "public", Provider: "p"})
	if err != nil || len(p.Requests) != 1 {
		t.Fatalf("%+v %v", p, err)
	}
	r, err := c.LogDetail(t.Context(), id)
	if err != nil || r.Status != "succeeded" || r.HTTP == nil || *r.HTTP != 200 || r.AttemptCount != 1 || r.Input == nil || *r.Input != 12 || r.ReasoningEffort == nil || *r.ReasoningEffort != "high" || r.Metrics.TimeToFirstTextMS != nil {
		t.Fatalf("%+v %v", r, err)
	}
	if len(p.Requests[0].Attempts) != 0 || len(r.Attempts) != 1 {
		t.Fatal("list/detail shape incorrect")
	}
	var export bytes.Buffer
	if err = c.ExportLogs(t.Context(), logbook.Query{}, &export); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(export.String(), "DO_NOT_STORE") || !strings.Contains(export.String(), id) {
		t.Fatal("export privacy/content", export.String())
	}
	cfg, err := c.LogSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Days = 14
	cfg, err = c.ConfigureLogs(t.Context(), cfg)
	if err != nil || cfg.Days != 14 {
		t.Fatal(cfg, err)
	}
	// Origin/instance protections remain active for logging writes/read POSTs.
	req, _ := http.NewRequest("PUT", f.control.URL+"/control/v1/logs/settings", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+f.owner)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode == 200 {
		t.Fatal("missing identity accepted")
	}
}
func TestProbeHTTP200ValidationFailureIsRecorded(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"output":[],"secret":"DO_NOT_STORE"}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "DO_NOT_STORE")
	provider, _ := f.store.Provider("p")
	probe, err := f.core.probe(t.Context(), provider, "default", "upstream")
	if err != nil || probe.Failure != "invalid_response" {
		t.Fatal(probe, err)
	}
	f.core.logs.Flush(t.Context())
	p, err := f.core.logs.List(t.Context(), logbook.Query{Source: "probe"})
	if err != nil || len(p.Requests) != 1 {
		t.Fatal(p, err)
	}
	r := p.Requests[0]
	if r.Status != "failed" || r.Code != "invalid_response" || r.Stage != "response_validate" || r.HTTP == nil || *r.HTTP != 200 || r.AttemptCount != 1 || r.ElapsedMS == nil {
		t.Fatalf("%+v", r)
	}
	r, err = f.core.logs.Detail(t.Context(), r.ID)
	if err != nil || r.Evidence["validation_reason"] != "missing_response_id" || r.Evidence["json_valid"] != "true" {
		t.Fatalf("validation diagnosis missing: %+v %v", r, err)
	}
	raw, _ := json.Marshal(r)
	if bytes.Contains(raw, []byte("DO_NOT_STORE")) {
		t.Fatal("body leaked")
	}
}
func TestStreamMeasurementsAreIndependentOfDisplay(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"DO_NOT_STORE\"}\n\n")
		w.(http.Flusher).Flush()
		time.Sleep(15 * time.Millisecond)
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"usage\":{\"input_tokens\":12,\"output_tokens\":9,\"output_tokens_details\":{\"reasoning_tokens\":0}}}}\n\n")
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"private","stream":true}`)
	io.Copy(io.Discard, resp.Body)
	f.core.logs.Flush(t.Context())
	r, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
	if err != nil || r.Status != "succeeded" || r.Metrics.TimeToFirstOutputMS == nil || r.Metrics.TimeToFirstTextMS == nil || r.Metrics.GenerationMS == nil || r.Metrics.OutputTextTokens == nil || *r.Metrics.OutputTextTokens != 9 {
		t.Fatalf("%+v %v", r, err)
	}
	if r.Metrics.CacheReadTokens != nil {
		t.Fatal("missing cache synthesized")
	}
}
func TestRetryTraceAndNoRoute(t *testing.T) {
	f := setup(t)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) }))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":3}}`)
	}))
	defer good.Close()
	f.add(t, "bad", bad.URL, "synthetic")
	f.add(t, "good", good.URL, "synthetic")
	f.model(t, state.Candidate{ProviderID: "bad", Model: "a"}, state.Candidate{ProviderID: "good", Model: "b"})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"x"}`)
	io.Copy(io.Discard, resp.Body)
	f.core.logs.Flush(t.Context())
	r, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
	if err != nil || r.Status != "succeeded" || r.AttemptCount != 2 || r.Attempts[0].Code != "rate_limited" || r.Input != nil {
		t.Fatal(r, err)
	}
	resp = request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"absent","input":"x"}`)
	io.Copy(io.Discard, resp.Body)
	f.core.logs.Flush(t.Context())
	r, err = f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
	if err != nil || r.Status != "failed" || r.AttemptCount != 0 || r.HTTP != nil {
		t.Fatal(r, err)
	}
}

// completionDisconnectWriter reproduces a client closing its connection as soon
// as it receives the terminal event, before the gateway finishes its bookkeeping.
type completionDisconnectWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
	marker string
	fail   bool
}

func (w *completionDisconnectWriter) Write(data []byte) (int, error) {
	if !bytes.Contains(data, []byte(w.marker)) {
		return w.ResponseRecorder.Write(data)
	}
	defer w.cancel()
	if w.fail {
		n, _ := w.ResponseRecorder.Write(data[:len(data)/2])
		return n, io.ErrClosedPipe
	}
	return w.ResponseRecorder.Write(data)
}

func TestCompletedResponseSurvivesClientDisconnect(t *testing.T) {
	for _, tc := range []struct {
		name, marker string
		stream, fail bool
	}{
		{name: "stream", marker: "response.completed", stream: true},
		{name: "json", marker: `"status":"completed"`},
		{name: "terminal_write_failure", marker: "response.completed", stream: true, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := `{"id":"r","status":"completed","output":[],"usage":{"input_tokens":12,"output_tokens":9,"output_tokens_details":{"reasoning_tokens":0}}}`
				if !tc.stream {
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, response)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"synthetic\"}\n\n")
				w.(http.Flusher).Flush()
				time.Sleep(15 * time.Millisecond)
				io.WriteString(w, `data: {"type":"response.completed","response":`+response+"}\n\n")
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "synthetic")
			f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			body, _ := json.Marshal(map[string]any{"model": "public", "input": "synthetic", "stream": tc.stream})
			req := httptest.NewRequest("POST", f.gateway.URL+"/v1/responses", bytes.NewReader(body)).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer "+f.token)
			w := &completionDisconnectWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel, marker: tc.marker, fail: tc.fail}
			f.core.Gateway().ServeHTTP(w, req)
			if ctx.Err() != context.Canceled {
				t.Fatal("completion did not trigger disconnect")
			}
			if err := f.core.logs.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			row, err := f.core.logs.Detail(t.Context(), w.Header().Get("X-Anchor-Request-ID"))
			if err != nil {
				t.Fatal(err)
			}
			if tc.fail {
				if row.Status != "cancelled" || row.Code != "client_cancelled" || row.Delivery != "partial" || row.Metrics.GenerationMS != nil {
					t.Fatalf("failed terminal write reported as complete: %+v", row)
				}
				return
			}
			if row.Status != "succeeded" || row.Code != "" || row.Delivery != "complete" || row.Attempts[0].Outcome != "succeeded" {
				t.Fatalf("disconnect overwrote a completed response: %+v", row)
			}
			if tc.stream && (!row.Metrics.TextIntervalComplete || row.Metrics.GenerationMS == nil || row.Metrics.OutputTextTokens == nil || *row.Metrics.OutputTextTokens != 9) {
				t.Fatalf("completed stream lost TPS measurements: %+v", row.Metrics)
			}
		})
	}
}

func TestTPSRequiresConfirmedTextMeasurements(t *testing.T) {
	start := time.Now()
	for _, tc := range []struct {
		name               string
		observation        protocol.Observation
		usage              protocol.Usage
		wantText, interval bool
	}{
		{name: "text", observation: protocol.Observation{FirstText: start, Terminal: start.Add(time.Second), Status: "completed"}, usage: protocol.Usage{Known: true, Output: 20, ReasoningKnown: true, Reasoning: 5}, wantText: true, interval: true},
		{name: "missing_usage", observation: protocol.Observation{FirstText: start, Terminal: start.Add(time.Second), Status: "completed"}, interval: true},
		{name: "missing_reasoning_usage", observation: protocol.Observation{FirstText: start, Terminal: start.Add(time.Second), Status: "completed"}, usage: protocol.Usage{Known: true, Output: 20}, interval: true},
		{name: "mixed_tool_output", observation: protocol.Observation{FirstText: start, Terminal: start.Add(time.Second), Status: "completed", NonText: true}, usage: protocol.Usage{Known: true, Output: 20, ReasoningKnown: true}, interval: true},
		{name: "tool_only", observation: protocol.Observation{Terminal: start.Add(time.Second), Status: "completed", NonText: true}, usage: protocol.Usage{Known: true, Output: 20, ReasoningKnown: true}},
		{name: "interrupted", observation: protocol.Observation{FirstText: start}, usage: protocol.Usage{Known: true, Output: 20, ReasoningKnown: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trace := &requestTrace{start: start}
			setLogMetrics(trace, tc.observation, tc.usage, "responses", true)
			metrics := trace.row.Metrics
			if (metrics.OutputTextTokens != nil) != tc.wantText || metrics.TextIntervalComplete != tc.interval {
				t.Fatalf("unexpected text measurements: %+v", metrics)
			}
			if tc.wantText && (*metrics.OutputTextTokens != 15 || metrics.GenerationMS == nil || *metrics.GenerationMS != 1000) {
				t.Fatalf("TPS must exclude reasoning tokens and first-text wait: %+v", metrics)
			}
		})
	}
}

func TestCancelledStreamProducesPartialLog(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"private\"}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", f.gateway.URL+"/v1/responses", strings.NewReader(`{"model":"public","input":"x","stream":true}`))
	req.Header.Set("Authorization", "Bearer "+f.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	id := resp.Header.Get("X-Anchor-Request-ID")
	buf := make([]byte, 1)
	if _, err = resp.Body.Read(buf); err != nil {
		t.Fatal(err)
	}
	cancel()
	resp.Body.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.core.logs.Flush(t.Context())
		r, e := f.core.logs.Detail(t.Context(), id)
		if e == nil && r.Status == "cancelled" {
			if r.Delivery != "partial" || r.Metrics.GenerationMS != nil || r.Input != nil {
				t.Fatal(r)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("cancelled stream was not finalized")
}

func TestGatewayLoggingLoad(t *testing.T) {
	if os.Getenv("ANCHOR_LOG_PERF") != "1" {
		t.Skip("opt-in load workload")
	}
	type result struct {
		p50, p95 time.Duration
		duration time.Duration
		dropped  int64
		heap     uint64
	}
	run := func(enabled bool) result {
		f := setup(t)
		up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			for i := 0; i < 16; i++ {
				io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"x\"}\n\n")
				w.(http.Flusher).Flush()
			}
			io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":16,\"output_tokens_details\":{\"reasoning_tokens\":0}}}}\n\n")
		}))
		defer up.Close()
		secret := "synthetic"
		if _, err := f.store.PutProvider(state.Provider{ID: "p", BaseURL: up.URL, Protocol: "responses", Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 8}, 0, &secret); err != nil {
			t.Fatal(err)
		}
		f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
		saved := f.core.logs
		if !enabled {
			f.core.logs = nil
			t.Cleanup(func() { saved.Close() })
		}
		f.core.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
		var wg sync.WaitGroup
		var mu sync.Mutex
		timings := make([]time.Duration, 0, 800)
		start := time.Now()
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 100 {
					begin := time.Now()
					req, _ := http.NewRequest("POST", f.gateway.URL+"/v1/responses", strings.NewReader(`{"model":"public","input":"x","stream":true}`))
					req.Header.Set("Authorization", "Bearer "+f.token)
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Error(err)
						return
					}
					buf := make([]byte, 1)
					_, err = resp.Body.Read(buf)
					first := time.Since(begin)
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if err != nil || resp.StatusCode != 200 {
						t.Errorf("stream %d %v", resp.StatusCode, err)
						return
					}
					mu.Lock()
					timings = append(timings, first)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		duration := time.Since(start)
		if len(timings) != 800 {
			t.Fatalf("incomplete load: %d", len(timings))
		}
		slices.Sort(timings)
		saved.Flush(t.Context())
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return result{timings[400], timings[760], duration, saved.Health().Dropped, m.HeapAlloc}
	}
	baseline := run(false)
	logged := run(true)
	t.Logf("800 requests x 16 deltas, 8 clients: baseline=%+v logged=%+v", baseline, logged)
	if logged.dropped != 0 {
		t.Fatal("paced load dropped logs", logged)
	}
	if logged.p95 > baseline.p95*2+10*time.Millisecond {
		t.Fatal("logging substantially regressed first-output latency", baseline, logged)
	}
}

func TestToolStreamOutputTPSIsPersistedAndExportedWithoutTextTPS(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"name\":\"exec\"}}\n\n")
		w.(http.Flusher).Flush()
		io.WriteString(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"DO_NOT_STORE\"}\n\n")
		w.(http.Flusher).Flush()
		time.Sleep(5 * time.Millisecond)
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"usage\":{\"input_tokens\":12,\"output_tokens\":9,\"output_tokens_details\":{\"reasoning_tokens\":0}}}}\n\n")
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"private","stream":true}`)
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if err := f.core.logs.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	c := logClient(t, f)
	r, err := c.LogDetail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
	if err != nil || r.Status != "succeeded" || r.Metrics.TimeToFirstOutputMS == nil || *r.Metrics.TimeToFirstOutputMS < 0 {
		t.Fatalf("tool stream must have a first output: %+v %v", r, err)
	}
	if r.Metrics.TimeToFirstTextMS != nil || r.Metrics.GenerationMS != nil || r.Metrics.OutputTextTokens != nil || r.Metrics.TextIntervalComplete {
		t.Fatalf("tool arguments must not become text measurements: %+v", r.Metrics)
	}
	if !r.Metrics.OutputIntervalComplete || r.Metrics.OutputGenerationMS == nil || *r.Metrics.OutputGenerationMS <= 0 || r.Metrics.NonReasoningOutputTokens == nil || *r.Metrics.NonReasoningOutputTokens != 9 {
		t.Fatalf("tool stream must retain output TPS measurements: %+v", r.Metrics)
	}
	var export bytes.Buffer
	if err := c.ExportLogs(t.Context(), logbook.Query{}, &export); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(export.String(), `"non_reasoning_output_tokens":9`) || !strings.Contains(export.String(), `"time_to_first_output_ms":`) || strings.Contains(export.String(), "DO_NOT_STORE") {
		t.Fatal("export must include timing, not tool arguments")
	}
}

func TestFirstOutputAndTextMetricsRemainIndependent(t *testing.T) {
	start := time.Now()
	tr := &requestTrace{start: start}
	setLogMetrics(tr, protocol.Observation{FirstOutput: start.Add(time.Second), FirstText: start.Add(2 * time.Second), Terminal: start.Add(3 * time.Second), Status: "completed", NonText: true}, protocol.Usage{Known: true, Output: 9, ReasoningKnown: true}, "responses", true)
	m := tr.row.Metrics
	if m.TimeToFirstOutputMS == nil || *m.TimeToFirstOutputMS != 1000 || m.TimeToFirstTextMS == nil || *m.TimeToFirstTextMS != 2000 || m.GenerationMS == nil || *m.GenerationMS != 1000 || m.OutputTextTokens != nil {
		t.Fatalf("first output must not shift text interval or synthesize text tokens: %+v", m)
	}
}
