package core

import (
	"net/http"

	"github.com/candyflosssss/Anchor/internal/apierror"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (s *Server) configureModelRouting(w http.ResponseWriter, r *http.Request) {
	var in catalog.ModelRouting
	if !decode(w, r, &in) {
		return
	}
	if in.ModelID != r.PathValue("id") || !state.ValidModelID(in.ModelID) || in.ExpectedRevision < 1 || in.Strategy == "" || !state.ValidModelStrategy(in.Strategy) {
		fail(w, apierror.InvalidRouting)
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
		fail(w, apierror.CandidateMembershipChanged)
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
			fail(w, apierror.UnknownCandidate)
			return
		}
		ordered = append(ordered, c)
		delete(candidates, ref)
	}
	// A source-order write is a permutation of the current Key cards, including
	// disabled and explicitly scoped missing Keys. It never changes mapping scope.
	if in.SourceOrder != nil {
		expected, err := s.modelSources(m)
		if err != nil {
			apiError(w, err)
			return
		}
		members := map[state.SourceRoute]bool{}
		for _, ref := range expected {
			members[ref] = true
		}
		if len(in.SourceOrder) != len(expected) {
			fail(w, apierror.CandidateMembershipChanged)
			return
		}
		for _, ref := range in.SourceOrder {
			if !members[ref] {
				fail(w, apierror.UnknownCandidate)
				return
			}
			delete(members, ref)
		}
	}
	m.SourceOrder = in.SourceOrder
	m.Candidates = ordered
	m.Strategy = in.Strategy
	m, err = s.Store.PutModel(m, in.ExpectedRevision)
	if err != nil {
		apiError(w, err)
		return
	}
	s.syncAgentMappings(w)
	writeJSON(w, 200, catalog.ModelRoutingState{ModelID: m.ID, Revision: m.Revision, Strategy: m.RoutingStrategy(), Order: in.Order, SourceOrder: m.SourceOrder})
}

// modelSources expands the current membership, without health filtering.
func (s *Server) modelSources(m state.Model) ([]state.SourceRoute, error) {
	out := []state.SourceRoute{}
	for _, c := range m.Candidates {
		ids := c.KeyIDs
		if len(ids) == 0 {
			keys, err := s.Store.ProviderKeys(c.ProviderID)
			if err != nil {
				return nil, err
			}
			for _, k := range keys {
				ids = append(ids, k.ID)
			}
			if len(ids) == 0 {
				ids = []string{"anonymous"}
			}
		}
		for _, id := range ids {
			out = append(out, state.SourceRoute{ProviderID: c.ProviderID, Model: c.Model, KeyID: id})
		}
	}
	return out, nil
}
