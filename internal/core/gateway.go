package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

type bindingKey struct{}

func (s *Server) Gateway() http.Handler {
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
	return s.guard("gateway", mux)
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
	requestID, err := state.Token()
	if err != nil {
		apiError(w, err)
		return
	}
	w.Header().Set("X-Anchor-Request-ID", requestID[:16])
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
	var modelID string
	if err = json.Unmarshal(request["model"], &modelID); err != nil || !state.ValidID(modelID) {
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
	model, err := s.Store.Model(modelID)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			fail(w, 404, "unknown_model", "public model is not configured")
		} else {
			apiError(w, err)
		}
		return
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
	for _, c := range model.Candidates {
		provider, err := s.Store.Provider(c.ProviderID)
		if err != nil {
			apiError(w, err)
			return
		}
		if !provider.Enabled || provider.Protocol != "responses" {
			continue
		}
		if !s.take(provider.ID, provider.MaxConcurrent) {
			continue
		}
		retry := s.forward(w, r, request, modelID, c, provider, stream)
		s.release(provider.ID)
		if !retry {
			return
		}
	}
	fail(w, 503, "no_available_candidate", "no enabled, supported and available candidate succeeded")
}
func (s *Server) forward(w http.ResponseWriter, r *http.Request, request map[string]json.RawMessage, public string, c state.Candidate, p state.Provider, stream bool) bool {
	started := time.Now()
	status := 0
	outcome := "failed"
	defer func() {
		s.Log.Info("model request", "request_id", w.Header().Get("X-Anchor-Request-ID"), "public_model", public, "upstream_model", c.Model, "provider", p.ID, "protocol", "responses->responses", "status", status, "outcome", outcome, "elapsed_ms", time.Since(started).Milliseconds(), "usage", "not_collected")
	}()
	raw, _ := json.Marshal(c.Model)
	request["model"] = raw
	body, err := json.Marshal(request)
	if err != nil {
		fail(w, 400, "invalid_request", "request could not be encoded")
		return false
	}
	target, err := responseURL(p.BaseURL)
	if err != nil {
		apiError(w, err)
		return false
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(p.TimeoutSeconds)*time.Second)
	defer cancel()
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
	if p.Secret != "" {
		up.Header.Set("Authorization", "Bearer "+p.Secret)
	}
	response, err := s.upstream.Do(up)
	if err != nil {
		// A transport timeout can follow upstream execution. Never replay an ambiguous request.
		if r.Context().Err() != nil {
			outcome = "canceled"
			return false
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = 504
			fail(w, 504, "upstream_timeout", "upstream timed out; execution and billing may be unknown")
		} else {
			status = 502
			fail(w, 502, "upstream_transport", "upstream transport failed; request was not replayed")
		}
		return false
	}
	defer response.Body.Close()
	status = response.StatusCode
	if status != 200 {
		// Do not reflect upstream error bodies, redirect locations, or authentication headers.
		switch status {
		case 429, 502, 503, 504:
			outcome = "candidate_rejected"
			return true
		}
		fail(w, 502, "upstream_rejected", "upstream rejected request; no automatic fallback for this status")
		return false
	}
	kind := response.Header.Get("Content-Type")
	mediaType, _, mediaErr := mime.ParseMediaType(kind)
	if stream {
		if mediaErr != nil || mediaType != "text/event-stream" {
			fail(w, 502, "invalid_upstream_response", "expected an SSE response")
			return false
		}
	} else {
		if mediaErr != nil || mediaType != "application/json" {
			fail(w, 502, "invalid_upstream_response", "expected a JSON response")
			return false
		}
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	// Once any downstream response is committed, no candidate replay is allowed.
	if stream {
		err = copyStream(w, response.Body)
	} else {
		err = copyBody(w, response.Body, false)
	}
	if err != nil {
		outcome = "stream_or_copy_interrupted"
		return false
	}
	outcome = "transport_completed"
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
