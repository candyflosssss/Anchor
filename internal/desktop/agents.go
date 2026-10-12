package desktop

import (
	"context"
	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
	"time"
)

// All Agent file/policy operations are core-owned; this bridge never scans or
// writes agent configuration and shares the CLI's authenticated client.
func (b *Bridge) AgentWorkspace() AgentWorkspaceResult {
	return b.agentWorkspaceCall(nil)
}
func (b *Bridge) EnrollAgent(in EnrollAgent) AgentWorkspaceResult {
	if !in.Approved || !state.ValidID(in.TargetID) || !state.ValidID(in.Model) || in.ExpectedRevision < 0 || in.ExpectedRevision >= MaxSafeRevision {
		return AgentWorkspaceResult{Contract: Contract, Problem: &Problem{"invalid"}}
	}
	return b.agentWorkspaceCall(func(ctx context.Context, cl *client.Client) (AgentWorkspace, error) { return cl.EnrollAgent(ctx, in) })
}
func (b *Bridge) ManageAgent(in ManageAgent) AgentWorkspaceResult {
	if !state.ValidID(in.BindingID) || in.ExpectedRevision < 1 || in.ExpectedRevision >= MaxSafeRevision || (in.Action != "sync" && in.Action != "disable" && in.Action != "enable") {
		return AgentWorkspaceResult{Contract: Contract, Problem: &Problem{"invalid"}}
	}
	return b.agentWorkspaceCall(func(ctx context.Context, cl *client.Client) (AgentWorkspace, error) {
		if _, err := cl.AgentAction(ctx, in.BindingID, in.Action, in.ExpectedRevision, ""); err != nil {
			return AgentWorkspace{}, err
		}
		if in.Action == "enable" {
			if _, err := cl.AgentAction(ctx, in.BindingID, "sync", in.ExpectedRevision+1, ""); err != nil {
				return AgentWorkspace{}, err
			}
		}
		return cl.AgentWorkspace(ctx)
	})
}
func (b *Bridge) agentWorkspaceCall(write func(context.Context, *client.Client) (AgentWorkspace, error)) AgentWorkspaceResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := AgentWorkspaceResult{Contract: Contract}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	var v AgentWorkspace
	if write == nil {
		v, err = cl.AgentWorkspace(ctx)
	} else {
		v, err = write(ctx, cl)
	}
	if err != nil {
		out.Problem = problem(err, write != nil)
		return out
	}
	for _, row := range v.Bindings {
		if row.Revision < 1 || row.Revision > MaxSafeRevision {
			out.Problem = &Problem{"read_error"}
			return out
		}
	}
	out.Data = &v
	return out
}
