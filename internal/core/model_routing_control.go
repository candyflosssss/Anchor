package core

import (
	"net/http"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (s *Server) configureModelRouting(w http.ResponseWriter, r *http.Request) {
	var in catalog.ModelRouting
	if !decode(w, r, &in) {
		return
	}
	if in.ModelID != r.PathValue("id") || !state.ValidID(in.ModelID) || in.ExpectedRevision < 1 || in.Strategy == "" || !state.ValidModelStrategy(in.Strategy) {
		fail(w, 400, "invalid", "invalid routing configuration")
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
	if len(in.Order) != len(m.Candidates) {
		fail(w, 400, "invalid", "candidate membership must remain unchanged")
		return
	}
	candidates := map[catalog.RouteCandidate]state.Candidate{}
	for _, c := range m.Candidates {
		candidates[catalog.RouteCandidate{ProviderID: c.ProviderID, Model: c.Model}] = c
	}
	ordered := make([]state.Candidate, 0, len(in.Order))
	for _, ref := range in.Order {
		c, ok := candidates[ref]
		if !ok {
			fail(w, 400, "invalid", "unknown or duplicate candidate")
			return
		}
		ordered = append(ordered, c)
		delete(candidates, ref)
	}
	m.Candidates = ordered
	m.Strategy = in.Strategy
	m, err = s.Store.PutModel(m, in.ExpectedRevision)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, 200, catalog.ModelRoutingState{ModelID: m.ID, Revision: m.Revision, Strategy: m.RoutingStrategy(), Order: in.Order})
}
