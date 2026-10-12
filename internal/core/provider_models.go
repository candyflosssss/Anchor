package core

import (
	"context"
	"net/http"
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
	keys, err := s.Store.ProviderKeys(p.ID)
	if err != nil {
		apiError(w, err)
		return
	}
	key := state.ProviderKey{ID: "anonymous", Enabled: true, Protocol: p.Protocol}
	if in.KeyID != "" || len(keys) > 0 {
		found := false
		for _, k := range keys {
			if k.ID == in.KeyID || (in.KeyID == "" && k.Enabled) {
				key = k
				found = true
				break
			}
		}
		if !found && in.KeyID != "" && !(len(keys) == 0 && in.KeyID == "anonymous") {
			apiError(w, state.ErrNotFound)
			return
		}
		if !found && len(keys) > 0 {
			writeJSON(w, 200, catalog.ProviderModels{ProviderID: p.ID, Revision: p.Revision, Models: []string{}, Failure: "authentication"})
			return
		}
	}
	p.Protocol = key.Protocol
	p.Secret = key.Secret
	out := catalog.ProviderModels{ProviderID: p.ID, KeyID: key.ID, Revision: p.Revision, Models: []string{}}
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
	out.Models, out.HTTPStatus, out.Failure = s.readProviderCatalog(ctx, p)
}
