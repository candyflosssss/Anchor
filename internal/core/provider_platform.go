package core

import (
	"context"
	"net/http"
	"time"

	"github.com/candyflosssss/Anchor/internal/apierror"
	"github.com/candyflosssss/Anchor/internal/egress"
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
			fail(w, apierror.ProviderIDMismatch)
			return
		}
		out, err := s.Store.ConfigureProviderPlatform(in)
		if err != nil {
			apiError(w, err)
			return
		}
		s.keyRateRefresh.notify()
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
			fail(w, apierror.InvalidPlatformQuery)
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
		if key.ID == in.KeyID || in.KeyID == "" && key.Enabled && (!found || key.ID == "default") {
			selected = key
			found = true
			if in.KeyID != "" || key.ID == "default" {
				break
			}
		}
	}
	if in.KeyID != "" && !found {
		return out, state.ErrNotFound
	}
	if !found && len(keys) > 0 {
		selected = state.ProviderKey{ID: "anonymous", Protocol: p.Protocol}
	}
	out.KeyID = selected.ID
	unlock, ok := s.tryPlatformQuery(in.ProviderID)
	if !ok {
		out.Failure = "busy"
		return out, nil
	}
	defer unlock()
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
	ctx = providerapi.WithRequestBudget(egress.WithKeyRoute(ctx, p.ID, selected.ID, found))
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
	in.KeyID = selected.ID
	cfg, detection, err := s.resolveProviderPlatform(ctx, p, selected, in.Operation == "detect-platform")
	if err != nil {
		return out, err
	}
	if in.Operation == "detect-platform" {
		return detection, nil
	}
	if detection.Failure != "" {
		out.Platform, out.Failure = "", detection.Failure
		out.Source, out.HTTPStatus = detection.Source, detection.HTTPStatus
		if in.Operation == "effective-rate" && in.RemoteKeyID == 0 && found {
			if err = s.cacheKeyRate(p, keys, selected.ID, &out); err != nil {
				return out, err
			}
		}
		return out, nil
	}
	adapter := providerapi.Adapter{HTTP: s.upstream, Config: cfg, BaseURL: p.BaseURL, Key: selected.Secret}
	result := adapter.Query(ctx, in)
	result.Source = append(detection.Source, result.Source...)
	if in.Operation == "effective-rate" && in.RemoteKeyID == 0 && found {
		if err = s.cacheKeyRate(p, keys, selected.ID, &result); err != nil {
			return out, err
		}
	}
	return result, nil
}

// Platform metadata operations are serialized per supplier, independently of
// inference. A concurrent read reports busy and never starts another detector.
func (s *Server) tryPlatformQuery(id string) (func(), bool) {
	v, _ := s.platformQueries.LoadOrStore(id, make(chan struct{}, 1))
	gate := v.(chan struct{})
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, true
	default:
		return nil, false
	}
}

// resolveProviderPlatform is called under the supplier metadata gate. Empty
// legacy configurations are initialized once on their first metadata query.
func (s *Server) resolveProviderPlatform(ctx context.Context, p state.Provider, key state.ProviderKey, force bool) (providerapi.Config, providerapi.Result, error) {
	cfg, err := s.Store.ProviderPlatform(p.ID)
	out := providerapi.Result{ProviderID: p.ID, Revision: p.Revision, KeyID: key.ID, Operation: "detect-platform", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: []string{}}
	if err != nil {
		return cfg, out, err
	}
	current, err := s.Store.Provider(p.ID)
	if err != nil {
		return cfg, out, err
	}
	if current.Revision != p.Revision {
		return cfg, out, state.ErrConflict
	}
	if !force {
		if cfg.Platform != "" && cfg.Platform != "auto" {
			return cfg, out, nil
		}
		if cfg.DetectedPlatform != "" {
			cfg.Platform = cfg.DetectedPlatform
			return cfg, out, nil
		}
		if cfg.DetectionFailure != "" {
			out.Failure = cfg.DetectionFailure
			return cfg, out, nil
		}
	}
	adapter := providerapi.Adapter{HTTP: s.upstream, Config: cfg, BaseURL: p.BaseURL, Key: key.Secret}
	out = adapter.Query(ctx, providerapi.Query{ProviderID: p.ID, ExpectedRevision: p.Revision, Operation: "detect-platform", KeyID: key.ID})
	if err = s.Store.SaveProviderPlatformDetection(p.ID, p.Revision, out.Platform, out.Failure, force); err != nil {
		return cfg, out, err
	}
	cfg.Platform = out.Platform
	return cfg, out, nil
}
