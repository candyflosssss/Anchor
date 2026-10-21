package core

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
)

type requestLogKey struct{}
type requestTrace struct {
	row                     logbook.Request
	start                   time.Time
	unknownUsage            bool
	unknownCost             bool
	written                 int64
	writeFailed             bool
	encryptedContextRetried bool
}

func trace(r *http.Request) *requestTrace {
	t, _ := r.Context().Value(requestLogKey{}).(*requestTrace)
	return t
}
func (s *Server) initLogs() {
	s.logOnce.Do(func() {
		logs, err := logbook.Open(s.Store.Dir)
		if err != nil {
			s.Log.Error("log storage unavailable")
			return
		}
		s.logs = logs
		s.Log = slog.New(logbook.NewHandler(s.Log.Handler(), logs))
	})
}
func (s *Server) recordRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || (r.URL.Path != "/v1/responses" && r.URL.Path != "/v1/messages" && r.URL.Path != "/v1/chat/completions") {
			next.ServeHTTP(w, r)
			return
		}
		id, e := state.Token()
		if e != nil {
			apiError(w, e)
			return
		}
		now := time.Now()
		t := &requestTrace{start: now, row: logbook.Request{ID: id[:16], StartedAtMS: now.UnixMilli(), Status: "pending", Source: "proxy", Stage: "request_parse", Delivery: "none", Attempts: []logbook.Attempt{}, Evidence: map[string]string{"sent_service_tier": "not_sent", "fast_triggered": "unknown"}}}
		logAttribution(r, t)
		switch r.URL.Path {
		case "/v1/messages":
			t.row.Protocol = "anthropic"
		case "/v1/chat/completions":
			t.row.Protocol = "chat-completions"
		default:
			t.row.Protocol = "responses"
		}
		w.Header().Set("X-Anchor-Request-ID", t.row.ID)
		if s.logs != nil {
			s.logs.Request(t.row)
		}
		r = r.WithContext(context.WithValue(r.Context(), requestLogKey{}, t))
		defer func() {
			ms := time.Since(now).Milliseconds()
			t.row.ElapsedMS = &ms
			// A client may disconnect immediately after receiving the terminal
			// event, while usage and log bookkeeping are still running. Keep a
			// response that was fully written successful; failed writes and
			// requests interrupted before completion remain cancellations.
			if r.Context().Err() != nil && (t.row.Status != "succeeded" || t.writeFailed) {
				t.row.Status = "cancelled"
				t.row.Code = "client_cancelled"
			}
			if t.row.Status == "pending" {
				t.row.Status = "failed"
				t.row.Code = "request_interrupted"
			}
			if t.writeFailed && r.Context().Err() == nil {
				t.row.Status = "failed"
				t.row.Code = "client_write_failed"
			}
			if t.row.Status == "succeeded" {
				t.row.Stage = "respond"
				t.row.Delivery = "complete"
			} else if t.written > 0 {
				t.row.Delivery = "partial"
			}
			if s.logs != nil {
				s.logs.Request(t.row)
				if t.row.Status != "succeeded" {
					s.logs.Event(logbook.NewEvent("warn", "core", "request."+t.row.Status, t.row.ID, map[string]string{"code": t.row.Code, "stage": t.row.Stage}))
				}
			}
		}()
		next.ServeHTTP(&logResponseWriter{ResponseWriter: w, trace: t}, r)
	})
}

type logResponseWriter struct {
	http.ResponseWriter
	trace  *requestTrace
	status int
}

func (w *logResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *logResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(data)
	if w.status == http.StatusOK {
		w.trace.written += int64(n)
	}
	if err != nil {
		w.trace.writeFailed = true
	}
	return n, err
}
func (w *logResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *logResponseWriter) logFailure(code string) {
	w.trace.row.Status = "failed"
	w.trace.row.Code = code
}
func (s *Server) observeRequest(r *http.Request, model string, stream bool, raw map[string]json.RawMessage, mapping state.Model) {
	if t := trace(r); t != nil {
		t.row.Model = model
		t.row.Stream = &stream
		t.row.Revision = mapping.Revision
		t.row.ReferenceCost = s.logReference(mapping)
		t.row.ReasoningObserved = raw["reasoning"] == nil
		if reasoning, ok := raw["reasoning"]; ok {
			var fields map[string]json.RawMessage
			if json.Unmarshal(reasoning, &fields) == nil {
				var effort string
				if json.Unmarshal(fields["effort"], &effort) == nil && knownReasoningEffort(effort) {
					t.row.ReasoningObserved = true
					t.row.ReasoningEffort = &effort
				}
			}
		}
	}
}
func setLogStage(r *http.Request, stage string) {
	if t := trace(r); t != nil {
		t.row.Stage = stage
	}
}
func setLogMetrics(t *requestTrace, obs protocol.Observation, u protocol.Usage, p string, success bool) {
	if t == nil {
		return
	}
	if t.row.Evidence == nil {
		t.row.Evidence = map[string]string{}
	}
	recordReturnedServiceTier(t.row.Evidence, obs.ServiceTier, obs.ServiceTierRaw)
	if !obs.FirstOutput.IsZero() {
		ms := obs.FirstOutput.Sub(t.start).Milliseconds()
		t.row.Metrics.TimeToFirstOutputMS = &ms
	}
	if !obs.FirstText.IsZero() {
		ms := obs.FirstText.Sub(t.start).Milliseconds()
		t.row.Metrics.TimeToFirstTextMS = &ms
	}
	t.row.GenerationStatus = obs.Status
	if u.CacheReadKnown {
		v := u.CacheRead
		t.row.Metrics.CacheReadTokens = &v
	}
	if u.ReasoningKnown {
		v := u.Reasoning
		t.row.Metrics.ReasoningTokens = &v
	}
	// Keep output and text measurements separate: tool arguments count as output,
	// but their token counts cannot be attributed to body text.
	if success && !obs.FirstOutput.IsZero() && !obs.Terminal.IsZero() && obs.Status == "completed" {
		duration := obs.Terminal.Sub(obs.FirstOutput).Milliseconds()
		if duration > 0 {
			t.row.Metrics.OutputGenerationMS = &duration
			t.row.Metrics.OutputIntervalComplete = true
			if u.Known && (u.ReasoningKnown || (p == "anthropic" && !obs.Reasoning)) && u.Reasoning >= 0 && u.Output >= u.Reasoning {
				output := u.Output - u.Reasoning
				t.row.Metrics.NonReasoningOutputTokens = &output
			}
		}
	}
	if success && !obs.FirstText.IsZero() && !obs.Terminal.IsZero() && obs.Status == "completed" {
		duration := obs.Terminal.Sub(obs.FirstText).Milliseconds()
		if duration > 0 {
			t.row.Metrics.GenerationMS = &duration
			t.row.Metrics.TextIntervalComplete = true
		}
		if u.Known && !obs.NonText && (u.ReasoningKnown || (p == "anthropic" && !obs.Reasoning)) {
			text := u.Output - u.Reasoning
			t.row.Metrics.OutputTextTokens = &text
		}
	}
}
func (t *requestTrace) addAttempt(a logbook.Attempt, u protocol.Usage) {
	if t == nil {
		return
	}
	t.priceAttempt(&a, u)
	if u.Known {
		if t.row.KnownUsage == nil {
			t.row.KnownUsage = &logbook.KnownUsage{}
		}
		t.row.KnownUsage.Input += u.Input
		t.row.KnownUsage.Output += u.Output
		t.row.KnownUsage.Attempts++
	}
	t.row.AttemptCount++
	if len(t.row.Attempts) < 64 {
		t.row.Attempts = append(t.row.Attempts, a)
	} else {
		t.row.AttemptsTruncated = true
	}
	t.row.UpstreamModel = a.Model
	t.row.Provider = a.Provider
	t.row.Key = a.Key
	t.row.HTTP = a.HTTP
	if !u.Known {
		t.unknownUsage = true
		t.row.Input = nil
		t.row.Output = nil
	}
	if u.Known && !t.unknownUsage {
		if t.row.Input == nil {
			v := int64(0)
			t.row.Input = &v
		}
		if t.row.Output == nil {
			v := int64(0)
			t.row.Output = &v
		}
		*t.row.Input += u.Input
		*t.row.Output += u.Output
	}
}
func knownReasoningEffort(s string) bool {
	switch s {
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return true
	}
	return false
}

// logIdentity accepts one bounded opaque label, excluding paths and known credentials.
// Invalid/ambiguous labels are ignored without rejecting an inference request.
func logIdentity(values []string, credentials ...string) string {
	if len(values) != 1 {
		return ""
	}
	value := values[0]
	if len(value) == 0 || len(value) > 128 {
		return ""
	}
	for i, c := range []byte(value) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		if i > 0 && (c == '-' || c == '_' || c == '.' || c == ':' || c == '@') {
			continue
		}
		return ""
	}
	for _, credential := range credentials {
		if value == credential {
			return ""
		}
	}
	return value
}
