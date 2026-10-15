package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/protocol"
	"io"
	"mime"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

type bindingKey struct{}
type protocolKey struct{}

func (s *Server) Gateway() http.Handler {
	s.initLogs()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		models, err := s.Store.Models()
		if err != nil {
			apiError(w, err)
			return
		}
		if b, ok := r.Context().Value(bindingKey{}).(state.Binding); ok {
			models, err = s.Store.BindingModels(b)
			if err != nil {
				apiError(w, err)
				return
			}
		}
		data := []map[string]any{}
		for _, m := range models {
			data = append(data, map[string]any{"id": m.ID, "object": "model", "owned_by": "anchor"})
		}
		writeJSON(w, 200, map[string]any{"object": "list", "data": data})
	})
	mux.HandleFunc("POST /v1/responses", s.responses)
	mux.HandleFunc("POST /v1/chat/completions", s.protocolHandler("chat-completions"))
	mux.HandleFunc("POST /v1/messages", s.protocolHandler("anthropic"))
	return s.guard("gateway", s.recordRequests(mux))
}
func (s *Server) take(id string, max int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[id] >= max {
		return false
	}
	s.active[id]++
	return true
}
func (s *Server) release(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[id]--
	if s.active[id] == 0 {
		delete(s.active, id)
	}
}
func (s *Server) responses(w http.ResponseWriter, r *http.Request) {
	var err error
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		fail(w, 429, "gateway_busy", "gateway concurrency limit reached")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		fail(w, 413, "request_too_large", "request exceeds limit or cannot be read")
		return
	}
	var request map[string]json.RawMessage
	if err = json.Unmarshal(body, &request); err != nil || request == nil {
		fail(w, 400, "invalid_json", "a JSON request object is required")
		return
	}
	observeRequestedServiceTier(r, request)
	var modelID string
	if err = json.Unmarshal(request["model"], &modelID); err != nil || !state.ValidModelID(modelID) {
		fail(w, 400, "invalid_model", "a public model ID is required")
		return
	}
	if b, ok := r.Context().Value(bindingKey{}).(state.Binding); ok {
		allowed, err := s.Store.BindingAllows(b, modelID)
		if err != nil {
			apiError(w, err)
			return
		}
		if !allowed {
			fail(w, 403, "binding_model_denied", "model is outside the binding route scope")
			return
		}
	}
	var stream bool
	if raw, ok := request["stream"]; ok {
		if err = json.Unmarshal(raw, &stream); err != nil {
			fail(w, 400, "invalid_stream", "stream must be boolean")
			return
		}
	}
	// Provider fallback cannot preserve server-side response IDs. Start with stateless requests.
	if raw, ok := request["previous_response_id"]; ok && string(raw) != "null" {
		fail(w, 400, "unsupported_state", "previous_response_id is not supported; submit full input history")
		return
	}
	if raw, ok := request["conversation"]; ok && string(raw) != "null" {
		fail(w, 400, "unsupported_state", "server-side conversations are not supported; submit full input history")
		return
	}
	if raw, ok := request["store"]; ok {
		var store bool
		if err = json.Unmarshal(raw, &store); err != nil || store {
			fail(w, 400, "unsupported_state", "this milestone supports store:false only")
			return
		}
	}
	if raw, ok := request["background"]; ok {
		var background bool
		if err = json.Unmarshal(raw, &background); err != nil || background {
			fail(w, 400, "unsupported_background", "background responses are not supported")
			return
		}
	}
	request["store"] = json.RawMessage("false")
	setLogStage(r, "route")
	model, err := s.Store.Model(modelID)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			fail(w, 404, "unknown_model", "public model is not configured")
		} else {
			apiError(w, err)
		}
		return
	}

	if model.ServiceTier != "" {
		request["service_tier"], _ = json.Marshal(model.ServiceTier)
		if t := trace(r); t != nil {
			t.row.Evidence["mapping_service_tier"] = model.ServiceTier
		}
	}
	observeRequest(r, modelID, stream, request, model.Revision)
	if s.logs != nil {
		if t := trace(r); t != nil {
			s.logs.Request(t.row)
		}
	}
	if stream && !model.Streaming {
		fail(w, 400, "unsupported_capability", "public model does not advertise streaming")
		return
	}
	if raw, ok := request["tools"]; ok && string(raw) != "null" {
		var tools []json.RawMessage
		if err = json.Unmarshal(raw, &tools); err != nil {
			fail(w, 400, "invalid_tools", "tools must be an array")
			return
		}
		if len(tools) > 0 && !model.Tools {
			fail(w, 400, "unsupported_capability", "public model does not advertise tools")
			return
		}
		for _, raw := range tools {
			if !agentExecutedTool(raw, false) {
				fail(w, 400, "unsupported_tool", "only Agent-executed function/custom tools, their namespaces and client tool_search are supported")
				return
			}
		}
	}
	affinity := routingAffinity(r.Header.Get("Authorization"), modelID, request)
	candidates, err := s.routeCandidatesFor(model, affinity)
	if err != nil {
		apiError(w, err)
		return
	}
	for _, c := range candidates {
		provider, err := s.Store.Provider(c.ProviderID)
		if err != nil {
			apiError(w, err)
			return
		}
		if !provider.Enabled {
			continue
		}
		if !s.take(provider.ID, provider.MaxConcurrent) {
			continue
		}
		keys, err := s.Store.ProviderKeys(provider.ID)
		if err != nil {
			s.release(provider.ID)
			apiError(w, err)
			return
		}
		tried := map[string]bool{}
		for n := 0; n < max(1, len(keys)); n++ {
			key, ok := s.chooseRouteKey(provider, tried, c.KeyIDs, s.preferredRouteKey(model, c, provider, affinity))
			if !ok {
				break
			}
			tried[key.ID] = true
			retry := s.forward(w, r, request, modelID, c, provider, key, stream, model, affinity)
			if !retry {
				s.release(provider.ID)
				return
			}
		}
		s.release(provider.ID)
	}
	setLogStage(r, "route")
	fail(w, 503, "no_available_candidate", "no enabled, supported and available candidate succeeded")
}
func (s *Server) protocolHandler(from string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			fail(w, 413, "request_too_large", "request exceeds limit")
			return
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) == nil && fields != nil {
			observeRequestedServiceTier(r, fields)
		}
		converted, err := protocol.ConvertRequest(raw, from, "responses")
		if err != nil {
			fail(w, 400, "unsupported_conversion", "unsupported field or content for protocol conversion")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(converted))
		r = r.WithContext(context.WithValue(r.Context(), protocolKey{}, from))
		s.responses(w, r)
	}
}
func authorizeUpstream(req *http.Request, p state.Provider, key string) {
	if p.Protocol == "anthropic" {
		if key != "" {
			req.Header.Set("x-api-key", key)
		}
		req.Header.Set("anthropic-version", "2023-06-01")
	} else if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
}
func (s *Server) forward(w http.ResponseWriter, r *http.Request, request map[string]json.RawMessage, public string, c state.Candidate, p state.Provider, key state.ProviderKey, stream bool, routeModel state.Model, affinity string) (retry bool) {
	// Resolve after key selection: the upstream model override wins over this key's default.
	p.Protocol = key.EffectiveProtocol(c.Model)
	started := time.Now()
	status := 0
	success := false
	attempted := false
	after := ""
	usage := protocol.Usage{}
	observation := protocol.Observation{}
	var upstreamHTTP *int
	code := ""
	setLogStage(r, "request_convert")
	if t := trace(r); t != nil {
		if _, exists := t.row.Evidence["model_revision"]; !exists {
			t.row.Evidence["model_revision"] = strconv.FormatInt(t.row.Revision, 10)
		}
		// Do not carry a prior candidate's reported tier into a failed retry.
		delete(t.row.Evidence, "upstream_service_tier")
		t.row.Revision = p.Revision
		t.row.Code = ""
	}
	defer func() {
		if !attempted { // Release a half-open reservation if local conversion was rejected.
			s.mu.Lock()
			if h := s.health[healthID(p, key.ID)]; h != nil {
				h.Probing = false
			}
			s.mu.Unlock()
			return
		}
		if r.Context().Err() == nil {
			s.finishKey(p, key.ID, status, after, success)
		} else {
			s.mu.Lock()
			if h := s.health[healthID(p, key.ID)]; h != nil {
				h.Probing = false
			}
			s.mu.Unlock()
		}
		t := trace(r)
		outcome := "failed"
		if success {
			elapsed := time.Since(started)
			if !observation.FirstText.IsZero() {
				elapsed = observation.FirstText.Sub(started)
			}
			s.rememberRoute(routeModel, c, p, key.ID, affinity, elapsed)
			outcome = "succeeded"
			if t != nil {
				t.row.Status = "succeeded"
				t.row.Code = ""
			}
		} else if retry {
			outcome = "retry"
		} else if r.Context().Err() != nil {
			outcome = "cancelled"
		}
		stage := "dispatch"
		if t != nil {
			stage = t.row.Stage
			if t.row.Code != "" {
				code = t.row.Code
			}
		}
		if !success && code == "" {
			code = "upstream_failed"
		}
		a := logbook.Attempt{Number: 1, Provider: p.ID, Key: key.ID, Model: c.Model, Protocol: p.Protocol, HTTP: upstreamHTTP, ElapsedMS: time.Since(started).Milliseconds(), Outcome: outcome, Code: code, Stage: stage}
		if usage.Known {
			in, out := usage.Input, usage.Output
			a.Input = &in
			a.Output = &out
		}
		if t != nil {
			a.Number = t.row.AttemptCount + 1
			t.addAttempt(a, usage)
			setLogMetrics(t, observation, usage, p.Protocol, success)
		}
		u := state.Usage{ProviderID: p.ID, Model: public, Requests: 1, ElapsedMS: time.Since(started).Milliseconds(), InputTokens: usage.Input, OutputTokens: usage.Output, CacheRead: usage.CacheRead, CacheWrite: usage.CacheWrite}
		if !success {
			u.Failures = 1
		}
		if usage.Known {
			u.Known = 1
		}
		if err := s.Store.RecordUsage(u); err != nil {
			s.Log.Error("usage persistence failed")
		}
		reportedTier := observation.ServiceTier
		if reportedTier == "" {
			reportedTier = "unknown"
		}
		s.Log.Info("model request", "public_model", public, "provider", p.ID, "key_id", key.ID, "upstream_model", c.Model, "protocol", p.Protocol, "status", status, "success", success, "elapsed_ms", u.ElapsedMS, "requested_service_tier", requestedServiceTier(request), "upstream_service_tier", reportedTier)
	}()
	request["model"], _ = json.Marshal(c.Model)
	raw, _ := json.Marshal(request)
	body, err := protocol.ConvertRequest(raw, "responses", p.Protocol)
	if err != nil {
		fail(w, 400, "unsupported_conversion", "candidate protocol cannot represent a request field or content")
		return false
	}
	base := s.pickURL(p)
	target := strings.TrimRight(base, "/") + protocol.Endpoint(p.Protocol)
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(p.TimeoutSeconds)*time.Second)
	defer cancel()
	var connected atomic.Bool
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { connected.Store(true) }})
	up, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		apiError(w, err)
		return false
	}
	up.Header.Set("Content-Type", "application/json")
	up.Header.Set("User-Agent", "Anchor/"+Version)
	if stream {
		up.Header.Set("Accept", "text/event-stream")
	} else {
		up.Header.Set("Accept", "application/json")
	}
	authorizeUpstream(up, p, key.Secret)
	attempted = true
	setLogStage(r, "dispatch")
	evidence := map[string]string{}
	resp, err := s.inferenceResponse(up, base, p.Protocol, evidence)
	if t := trace(r); t != nil {
		// Endpoint evidence describes the current candidate, not an earlier retry.
		for _, name := range []string{"endpoint_resolution", "endpoint_requests", "initial_response_kind", "initial_http", "response_kind", "upstream_content_type"} {
			delete(t.row.Evidence, name)
		}
		for name, value := range evidence {
			t.row.Evidence[name] = value
		}
	}
	if err != nil {
		if r.Context().Err() != nil {
			return false
		}
		if !connected.Load() {
			code = "upstream_connect"
			return true
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = 504
			fail(w, 504, "upstream_timeout", "upstream timed out; execution may be unknown")
		} else {
			status = 502
			fail(w, 502, "upstream_transport", "upstream transport failed; request was not replayed")
		}
		return false
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	upstreamHTTP = &status
	setLogStage(r, "response_validate")
	after = resp.Header.Get("Retry-After")
	setLogStage(r, "upstream_response")
	if status != 200 {
		switch status {
		case 401, 403, 429, 502, 503, 504:
			code = "upstream_unavailable"
			if status == 429 {
				code = "rate_limited"
			}
			if status == 401 || status == 403 {
				code = "authentication"
			}
			return true
		}
		fail(w, 502, "upstream_rejected", "upstream rejected request")
		return false
	}
	setLogStage(r, "response_validate")
	if evidence["response_kind"] == "html" {
		code = "unexpected_html"
		fail(w, 502, code, "upstream returned an HTML page instead of an API response; check the API base URL (usually ending in /v1)")
		return false
	}
	media, _, e := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	expected := "application/json"
	if stream {
		expected = "text/event-stream"
	}
	if e != nil || media != expected {
		fail(w, 502, "invalid_upstream_response", "unexpected upstream content type")
		return false
	}
	to, _ := r.Context().Value(protocolKey{}).(string)
	if to == "" {
		to = "responses"
	}
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(200)
		setLogStage(r, "stream")
		usage, err = protocol.StreamObserved(w, resp.Body, p.Protocol, to, public, &observation)
		if err != nil {
			code = "stream_interrupted"
			if t := trace(r); t != nil {
				t.row.Status = "failed"
				t.row.Code = code
			}
		}
		success = err == nil
		return false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil || len(data) > 16<<20 {
		fail(w, 502, "invalid_upstream_response", "upstream response incomplete or oversized")
		return false
	}
	m, decodeErr := protocol.Decode(data)
	if decodeErr == nil {
		observation.ServiceTier = protocol.ReadServiceTier(m, p.Protocol)
	}
	if p.Protocol == to {
		if decodeErr != nil {
			fail(w, 502, "invalid_upstream_response", "invalid upstream JSON")
			return false
		}
		usage = protocol.ReadUsage(m, p.Protocol)
		observation.Status = safeGenerationStatus(m, p.Protocol)
		if m["error"] != nil || m["status"] == "failed" {
			fail(w, 502, "upstream_failed", "upstream generation failed")
			return false
		}
		setLogStage(r, "respond")
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(data)
		success = err == nil
		return false
	}
	result, err := protocol.ParseResponse(data, p.Protocol)
	if err != nil {
		fail(w, 502, "unsupported_upstream_response", "upstream returned unsupported content")
		return false
	}
	usage = result.Usage
	observation.Status = "completed"
	if result.Finish == "length" {
		observation.Status = "incomplete"
	}
	if to == "anthropic" && !usage.Known {
		fail(w, 502, "unsupported_upstream_response", "upstream omitted usage required by Messages")
		return false
	}
	setLogStage(r, "respond")
	result.Model = public
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(result.JSON(to))
	success = err == nil
	return false
}
func copyStream(w http.ResponseWriter, body io.Reader) error { return copyBody(w, body, true) }
func copyBody(w http.ResponseWriter, body io.Reader, flush bool) error {
	controller := http.NewResponseController(w)
	buf := make([]byte, 32<<10)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			if e := controller.SetWriteDeadline(time.Now().Add(15 * time.Second)); e != nil && !errors.Is(e, http.ErrNotSupported) {
				return e
			}
			if _, e := w.Write(buf[:n]); e != nil {
				return e
			}
			if flush {
				if e := controller.Flush(); e != nil {
					return e
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
