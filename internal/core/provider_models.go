package core

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/candyflosssss/Anchor/internal/apierror"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/egress"
	"github.com/candyflosssss/Anchor/internal/providerapi"
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
		fail(w, apierror.ProviderRevisionRequired)
		return
	}
	unlock, ok := s.tryPlatformQuery(in.ProviderID)
	if !ok {
		writeJSON(w, 200, catalog.ProviderModels{ProviderID: in.ProviderID, Revision: in.ExpectedRevision, Models: []string{}, Entries: []catalog.ModelEntry{}, Keys: []catalog.CatalogKey{}, Failure: "busy"})
		return
	}
	defer unlock()
	p, err := s.Store.Provider(in.ProviderID)
	if err != nil {
		apiError(w, err)
		return
	}
	if p.Revision != in.ExpectedRevision {
		apiError(w, state.ErrConflict)
		return
	}
	allKeys, err := s.Store.ProviderKeys(p.ID)
	if err != nil {
		apiError(w, err)
		return
	}
	keys := make([]state.ProviderKey, 0, len(allKeys))
	if in.KeyID != "" && !(len(allKeys) == 0 && in.KeyID == "anonymous") &&
		!slices.ContainsFunc(allKeys, func(k state.ProviderKey) bool { return k.ID == in.KeyID }) {
		apiError(w, state.ErrNotFound)
		return
	}
	for _, k := range allKeys {
		if k.Enabled && (in.KeyID == "" || k.ID == in.KeyID) {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 && len(allKeys) == 0 {
		keys = []state.ProviderKey{{ID: "anonymous", Enabled: true, Protocol: p.Protocol}}
	}
	cfg, err := s.Store.ProviderPlatform(p.ID)
	if err != nil {
		apiError(w, err)
		return
	}
	current, err := s.Store.Provider(p.ID)
	if err != nil {
		apiError(w, err)
		return
	}
	if current.Revision != p.Revision {
		apiError(w, state.ErrConflict)
		return
	}
	out := catalog.ProviderModels{ProviderID: p.ID, KeyID: in.KeyID, Revision: p.Revision, Models: []string{}, Entries: []catalog.ModelEntry{}, Keys: []catalog.CatalogKey{}}
	if len(keys) == 1 && keys[0].ID == "anonymous" {
		out.KeyID = "anonymous"
	}
	start := time.Now()
	var cacheErr error
	defer func() {
		if cacheErr != nil {
			apiError(w, cacheErr)
			return
		}
		out.ElapsedMS = time.Since(start).Milliseconds()
		writeJSON(w, 200, out)
	}()
	if len(keys) == 0 {
		out.Failure = "no_enabled_keys"
		return
	}
	// Directory reads only use enabled Keys and never mutate provider configuration.
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
	if len(keys) > catalog.MaxKeys {
		out.Failure = "too_large"
		return
	}
	byModel := map[string][]string{}
	budget := &catalogBudget{}
	successes := 0
	for _, key := range keys {
		provider := p
		provider.Protocol, provider.Secret = key.Protocol, key.Secret
		ids, status, failure := s.readProviderCatalogBudget(egress.WithKeyRoute(ctx, p.ID, key.ID, key.Secret != ""), provider, budget)
		// Reject echoes of any saved credential, including another Key's secret.
		for _, id := range ids {
			for _, k := range allKeys {
				if k.Secret != "" && strings.Contains(id, k.Secret) {
					failure = "invalid_response"
				}
			}
			if cfg.UserToken != "" && strings.Contains(id, cfg.UserToken) {
				failure = "invalid_response"
			}
		}
		out.Keys = append(out.Keys, catalog.CatalogKey{ID: key.ID, Enabled: key.Enabled, Protocol: key.Protocol, HTTPStatus: status, Failure: failure, RateFailure: "unknown"})
		if failure != "" {
			if out.Failure == "" {
				out.Failure, out.HTTPStatus = failure, status
			}
			continue
		}
		successes++
		for _, id := range ids {
			byModel[id] = append(byModel[id], key.ID)
		}
	}
	out.Partial = successes > 0 && successes < len(keys)
	if successes > 0 {
		out.Failure, out.HTTPStatus = "", http.StatusOK
	}
	if len(byModel) > catalog.MaxModels {
		out.Failure = "too_large"
		out.Partial = false
		return
	}
	for id := range byModel {
		out.Models = append(out.Models, id)
	}
	slices.Sort(out.Models)
	for _, id := range out.Models {
		out.Entries = append(out.Entries, catalog.ModelEntry{ID: id, KeyIDs: byModel[id]})
	}
	// Match the management client's 1 MiB response limit before additional reads.
	encoded, _ := json.Marshal(out)
	if len(encoded) > (1<<20)-(192<<10) {
		out.Models = []string{}
		out.Entries = []catalog.ModelEntry{}
		out.Failure = "too_large"
		out.Partial = false
		return
	}
	// Read catalogs first so missing/slow billing permissions never hide models.
	firstRateContext := providerapi.WithRequestBudget(ctx)
	resolved, detection, resolveErr := s.resolveProviderPlatform(egress.WithKeyRoute(firstRateContext, p.ID, keys[0].ID, keys[0].Secret != ""), p, keys[0], false)
	if resolveErr != nil {
		cacheErr = resolveErr
		return
	}
	for i, key := range keys {
		if detection.Failure != "" {
			out.Keys[i].RateFailure = detection.Failure
			continue
		}
		adapter := providerapi.Adapter{HTTP: s.upstream, Config: resolved, BaseURL: p.BaseURL, Key: key.Secret}
		rateContext := ctx
		if i == 0 {
			rateContext = firstRateContext
		}
		rate := adapter.QueryKeyRate(egress.WithKeyRoute(rateContext, p.ID, key.ID, key.Secret != ""))
		if cacheErr = s.cacheKeyRate(p, allKeys, key.ID, &rate); cacheErr != nil {
			return
		}
		out.Keys[i].RateFailure = rate.Failure
		if rate.EffectiveRate != nil {
			v := rate.EffectiveRate
			valid := !strings.ContainsFunc(v.Group+v.GroupName, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) })
			if v.Multiplier != nil && len(v.Multiplier.String()) > 128 {
				valid = false
			}
			for _, credential := range allKeys {
				if credential.Secret != "" && (strings.Contains(v.Group, credential.Secret) || strings.Contains(v.GroupName, credential.Secret)) {
					valid = false
				}
			}
			if !valid {
				out.Keys[i].RateFailure = "invalid_response"
				continue
			}
			out.Keys[i].Group, out.Keys[i].GroupName, out.Keys[i].RateBasis = v.Group, v.GroupName, v.Basis
			if v.Multiplier != nil {
				value := v.Multiplier.String()
				out.Keys[i].Multiplier = &value
			}
		}
	}
}
