package core

import (
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
	"net/http"
)

func modelSettingsState(m state.Model) catalog.ModelSettingsState {
	out := catalog.ModelSettingsState{ModelID: m.ID, Revision: m.Revision, DisplayName: m.DisplayName, CandidateCount: len(m.Candidates)}
	if m.Settings != nil {
		out.Configuration = m.Settings.Configuration
		out.Reference = m.Settings.Reference
	}
	// These existing shared mapping flags remain authoritative.
	out.Configuration.Capabilities.ToolCall = &m.Tools
	out.Configuration.Streaming = &m.Streaming
	if out.Configuration.SupportedReasoning == nil {
		out.Configuration.SupportedReasoning = []modeldev.ReasoningOption{}
	}
	for i := range out.Configuration.SupportedReasoning {
		if out.Configuration.SupportedReasoning[i].Values == nil {
			out.Configuration.SupportedReasoning[i].Values = []*string{}
		}
	}
	if out.Configuration.Capabilities.InputModalities == nil {
		out.Configuration.Capabilities.InputModalities = []string{}
	}
	if out.Configuration.Capabilities.OutputModalities == nil {
		out.Configuration.Capabilities.OutputModalities = []string{}
	}
	return out
}
func (s *Server) getModelSettings(w http.ResponseWriter, r *http.Request) {
	if !s.identity(w, r) {
		return
	}
	if !state.ValidID(r.PathValue("id")) {
		apiError(w, state.ErrInvalid)
		return
	}
	m, err := s.Store.Model(r.PathValue("id"))
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, 200, modelSettingsState(m))
}
func (s *Server) saveModelSettings(w http.ResponseWriter, r *http.Request) {
	if !s.identity(w, r) {
		return
	}
	var in catalog.SaveModelSettings
	if !decode(w, r, &in) {
		return
	}
	if in.ModelID != r.PathValue("id") || in.Validate() != nil {
		apiError(w, state.ErrInvalid)
		return
	}
	m, err := s.Store.Model(in.ModelID)
	if err != nil {
		apiError(w, err)
		return
	}
	if m.Revision != in.ExpectedRevision {
		apiError(w, state.ErrConflict)
		return
	}
	settings := state.ModelSettings{Configuration: in.Configuration}
	if in.ReferenceAction == "keep" && m.Settings != nil {
		settings.Reference = m.Settings.Reference
	}
	if in.ReferenceAction == "set" {
		snapshot, e := s.modelCatalog.Query(r.Context(), modeldev.ModelCatalogQuery{ProviderID: in.ReferenceProviderID, ModelID: in.ReferenceModelID})
		if e != nil {
			modelCatalogError(w, e)
			return
		}
		if snapshot.Stale {
			fail(w, 502, "catalog_unavailable", "refresh failed; reference was not replaced")
			return
		}
		settings.Reference = &snapshot
	}
	m.Settings = &settings
	m.DisplayName = in.DisplayName
	if settings.Configuration.Capabilities.ToolCall != nil {
		m.Tools = *settings.Configuration.Capabilities.ToolCall
	}
	if settings.Configuration.Streaming != nil {
		m.Streaming = *settings.Configuration.Streaming
	}
	m, err = s.Store.PutModel(m, in.ExpectedRevision)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, 200, modelSettingsState(m))
}
