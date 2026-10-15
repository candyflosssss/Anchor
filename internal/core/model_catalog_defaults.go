package core

import (
	"context"
	"errors"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/modelconfig"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
	"time"
)

// Creation is best-effort and bounded: an unavailable/ambiguous catalog must
// never prevent mapping creation or silently guess across different candidates.
func (s *Server) initializeModelCatalog(ctx context.Context, m *state.Model) {
	if m.Settings != nil || len(m.Candidates) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	snapshot, err := s.resolveModelCatalog(ctx, *m)
	if err != nil {
		return
	}
	cfg := modelconfig.FillCatalog(modelconfig.ModelConfiguration{}, snapshot.Models[0])
	// Explicit mapping flags are authoritative; models.dev does not declare streaming.
	cfg.Capabilities.ToolCall, cfg.Streaming = &m.Tools, &m.Streaming
	settings := &state.ModelSettings{Configuration: cfg, Reference: &snapshot}
	if m.ServiceTier != "" {
		settings.Reference = nil
	}
	if settings.Validate() == nil {
		m.Settings = settings
	}
}

// Resolve upstream candidates, never the public alias or the local provider name.
func (s *Server) resolveModelCatalog(ctx context.Context, m state.Model) (modeldev.ModelCatalog, error) {
	if len(m.Candidates) == 0 {
		return modeldev.ModelCatalog{}, modeldev.ErrNotFound
	}
	var snapshot modeldev.ModelCatalog
	for i, candidate := range m.Candidates {
		v, err := s.modelCatalog.Resolve(ctx, candidate.Model)
		if err != nil {
			return modeldev.ModelCatalog{}, err
		}
		if i > 0 && (snapshot.Models[0].ProviderID != v.Models[0].ProviderID || snapshot.Models[0].ID != v.Models[0].ID) {
			return modeldev.ModelCatalog{}, modeldev.ErrNotFound
		}
		snapshot = v
	}
	return snapshot, nil
}

// Existing mappings also need defaults, including partially configured mappings
// and ones created while the catalog was unavailable. GET only prepares a draft;
// the existing revision-checked save remains the sole write path.
func (s *Server) previewModelCatalog(ctx context.Context, m state.Model) catalog.ModelSettingsState {
	out := modelSettingsState(m)
	snapshot := out.Reference
	out.DefaultsStatus = "reference"
	if snapshot == nil {
		ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		v, err := s.resolveModelCatalog(ctx, m)
		if err != nil {
			out.DefaultsStatus = "unavailable"
			if errors.Is(err, modeldev.ErrNotFound) {
				out.DefaultsStatus = "unmatched"
			}
			return out
		}
		snapshot = &v
		out.DefaultsStatus = "matched"
	}
	cfg := modelconfig.FillCatalog(out.Configuration, snapshot.Models[0])
	if cfg.Validate() != nil {
		out.DefaultsStatus = "incompatible"
		return out
	}
	out.Configuration = cfg
	// Standard prices do not establish a speed variant's prices. Existing
	// explicitly chosen references are retained, but never inferred for speed.
	if m.ServiceTier == "" {
		out.Reference = snapshot
	}
	return out
}
