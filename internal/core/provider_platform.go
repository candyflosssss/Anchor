package core

import (
	"context"
	"net/http"
	"time"

	"github.com/candyflosssss/Anchor/internal/providerapi"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (s *Server) providerPlatformRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /control/v1/providers/{id}/platform", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.Store.ProviderPlatformSettings(r.PathValue("id"))
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("PUT /control/v1/providers/{id}/platform", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in providerapi.Configure
		if !decode(w, r, &in) {
			return
		}
		if in.ProviderID != r.PathValue("id") {
			fail(w, 400, "invalid_request", "provider IDs must match")
			return
		}
		out, err := s.Store.ConfigureProviderPlatform(in)
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /control/v1/providers/{id}/query", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in providerapi.Query
		if !decode(w, r, &in) {
			return
		}
		if in.ProviderID != r.PathValue("id") || !state.ValidProviderID(in.ProviderID) || (in.KeyID != "" && !state.ValidID(in.KeyID)) || in.Validate() != nil {
			fail(w, 400, "invalid_request", "invalid platform query")
			return
		}
		out, err := s.QueryProvider(r.Context(), in)
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
}

// QueryProvider is the single core entry point. It owns authorization selection,
// provider revision, concurrency and deadline for all read-only platform APIs.
func (s *Server) QueryProvider(parent context.Context, in providerapi.Query) (providerapi.Result, error) {
	out := providerapi.Result{ProviderID: in.ProviderID, RemoteKeyID: in.RemoteKeyID, Revision: in.ExpectedRevision, Operation: in.Operation, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: []string{}}
	if !state.ValidProviderID(in.ProviderID) || (in.KeyID != "" && !state.ValidID(in.KeyID)) || in.Validate() != nil {
		return out, state.ErrInvalid
	}
	p, err := s.Store.Provider(in.ProviderID)
	if err != nil {
		return out, err
	}
	if p.Revision != in.ExpectedRevision {
		return out, state.ErrConflict
	}
	cfg, err := s.Store.ProviderPlatform(p.ID)
	if err != nil {
		return out, err
	}
	out.Platform = cfg.Platform
	keys, err := s.Store.ProviderKeys(p.ID)
	if err != nil {
		return out, err
	}
	// Reject a configuration changed while the provider/credential snapshot was read.
	current, err := s.Store.Provider(p.ID)
	if err != nil {
		return out, err
	}
	if current.Revision != p.Revision {
		return out, state.ErrConflict
	}
	selected := state.ProviderKey{ID: "anonymous", Protocol: p.Protocol, Enabled: true}
	found := false
	for _, key := range keys {
		if key.ID == in.KeyID || in.KeyID == "" && key.Enabled {
			selected = key
			found = true
			break
		}
	}
	if in.KeyID != "" && !found {
		return out, state.ErrNotFound
	}
	if !found && len(keys) > 0 {
		selected = state.ProviderKey{ID: "anonymous", Protocol: p.Protocol}
	}
	out.KeyID = selected.ID
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		out.Failure = "busy"
		return out, nil
	}
	if !s.take(p.ID, p.MaxConcurrent) {
		out.Failure = "busy"
		return out, nil
	}
	defer s.release(p.ID)
	ctx, cancel := context.WithTimeout(parent, min(time.Duration(p.TimeoutSeconds)*time.Second, 8*time.Second))
	defer cancel()
	if in.Operation == "models" {
		if !found && len(keys) > 0 {
			out.Failure = "auth_required"
			return out, nil
		}
		p.Secret = selected.Secret
		p.Protocol = selected.Protocol
		out.Models, out.HTTPStatus, out.Failure = s.readProviderCatalog(ctx, p)
		out.Source = []string{"model_catalog"}
		return out, nil
	}
	adapter := providerapi.Adapter{HTTP: s.upstream, Config: cfg, BaseURL: p.BaseURL, Key: selected.Secret}
	in.KeyID = selected.ID
	return adapter.Query(ctx, in), nil
}
