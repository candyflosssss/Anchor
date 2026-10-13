package desktop

import (
	"context"
	"time"
)

func (b *Bridge) ModelCatalog(in ModelCatalogQuery) ModelCatalogResult {
	out := ModelCatalogResult{Contract: Contract}
	if in.Validate() != nil {
		out.Problem = &Problem{"invalid"}
		return out
	}
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	data, err := cl.ModelCatalog(ctx, in)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	out.Data = &data
	return out
}
func (b *Bridge) EstimateModelCost(in CatalogEstimateInput) CatalogEstimateResult {
	out := CatalogEstimateResult{Contract: Contract}
	if in.Validate() != nil {
		out.Problem = &Problem{"invalid"}
		return out
	}
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	data, err := cl.EstimateModelCost(ctx, in)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	out.Data = &data
	return out
}
