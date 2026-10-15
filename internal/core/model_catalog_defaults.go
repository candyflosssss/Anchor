package core

import (
	"context"
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
	var snapshot modeldev.ModelCatalog
	for i, candidate := range m.Candidates {
		v, err := s.modelCatalog.Resolve(ctx, candidate.Model)
		if err != nil {
			return
		}
		if i > 0 && (snapshot.Models[0].ProviderID != v.Models[0].ProviderID || snapshot.Models[0].ID != v.Models[0].ID) {
			return
		}
		snapshot = v
	}
	cfg := modelconfig.FillCatalog(modelconfig.ModelConfiguration{}, snapshot.Models[0])
	// Explicit mapping flags are authoritative; models.dev does not declare streaming.
	cfg.Capabilities.ToolCall, cfg.Streaming = &m.Tools, &m.Streaming
	settings := &state.ModelSettings{Configuration: cfg, Reference: &snapshot}
	if settings.Validate() == nil {
		m.Settings = settings
	}
}
