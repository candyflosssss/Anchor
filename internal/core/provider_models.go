package core

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (s *Server) discoverProviderModels(w http.ResponseWriter, r *http.Request) {
	if !s.identity(w, r) {
		return
	}
	var in catalog.DiscoverProviderModels
	if !decode(w, r, &in) {
		return
	}
	if !state.ValidProviderID(in.ProviderID) || in.ProviderID != r.PathValue("id") || in.ExpectedRevision < 1 {
		fail(w, 400, "invalid_request", "saved provider ID and revision are required")
		return
	}
	p, err := s.Store.Provider(in.ProviderID)
	if err != nil {
		apiError(w, err)
		return
	}
	if p.Revision != in.ExpectedRevision {
		apiError(w, state.ErrConflict)
		return
	}
	out := catalog.ProviderModels{ProviderID: p.ID, Revision: p.Revision, Models: []string{}}
	start := time.Now()
	defer func() { out.ElapsedMS = time.Since(start).Milliseconds(); writeJSON(w, 200, out) }()
	// Explicit management reads may inspect disabled providers, but never bypass
	// the gateway's global/provider concurrency limits or mutate their config.
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		out.Failure = "busy"
		return
	}
	if !s.take(p.ID, p.MaxConcurrent) {
		out.Failure = "busy"
		return
	}
	defer s.release(p.ID)
	timeout := min(time.Duration(p.TimeoutSeconds)*time.Second, 8*time.Second)
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.BaseURL, "/")+"/models", nil)
	if err != nil {
		out.Failure = "network"
		return
	}
	req.Header.Set("Accept", "application/json")
	keys, e := s.Store.ProviderKeys(p.ID)
	if e != nil {
		out.Failure = "network"
		return
	}
	p.Secret = ""
	for _, k := range keys {
		if k.Enabled {
			p.Secret = k.Secret
			break
		}
	}
	if len(keys) > 0 && p.Secret == "" {
		out.Failure = "authentication"
		return
	}
	authorizeUpstream(req, p, p.Secret)
	resp, err := s.upstream.Do(req)
	if err != nil {
		out.Failure = "network"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			out.Failure = "timeout"
		}
		return
	}
	defer resp.Body.Close()
	out.HTTPStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		// Never read/reflect an upstream error body or follow its redirects.
		switch resp.StatusCode {
		case 401, 403:
			out.Failure = "authentication"
		case 404, 405:
			out.Failure = "unsupported"
		case 429:
			out.Failure = "rate_limited"
		default:
			out.Failure = "http_error"
		}
		return
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, catalog.MaxBytes+1))
	if err != nil {
		out.Failure = "network"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			out.Failure = "timeout"
		}
		return
	}
	if len(raw) > catalog.MaxBytes {
		out.Failure = "too_large"
		return
	}
	models, err := catalog.Parse(raw, p.Secret)
	if err != nil {
		out.Failure = "invalid_response"
		return
	}
	out.Models = models
}
