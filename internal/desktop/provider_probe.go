package desktop

import (
	"context"
	"github.com/candyflosssss/Anchor/internal/state"
	"time"
)

func (b *Bridge) ProviderProbe(in ProbeRequest) ProbeProgressResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := ProbeProgressResult{Contract: Contract}
	if !state.ValidProviderID(in.ProviderID) || in.ExpectedRevision < 1 || in.ExpectedRevision >= MaxSafeRevision {
		out.Problem = &Problem{"invalid"}
		return out
	}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	data, err := cl.ProviderProbe(ctx, in)
	if err != nil {
		out.Problem = problem(err, in.Action == "start")
		return out
	}
	out.Data = &data
	return out
}
