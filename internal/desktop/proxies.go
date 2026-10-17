package desktop

import (
	"context"
	"time"
)

func (b *Bridge) ProxyWorkspace() ProxyWorkspaceResult { return b.proxyCall(nil) }
func (b *Bridge) ManageProxy(in ProxyAction) ProxyWorkspaceResult {
	if !in.Approved || in.ExpectedRevision < 1 || in.ExpectedRevision >= MaxSafeRevision {
		return ProxyWorkspaceResult{Contract: Contract, Problem: &Problem{"invalid"}}
	}
	return b.proxyCall(&in)
}
func (b *Bridge) proxyCall(in *ProxyAction) ProxyWorkspaceResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := ProxyWorkspaceResult{Contract: Contract}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	var data ProxyWorkspace
	if in == nil {
		data, err = cl.ProxyWorkspace(ctx)
	} else {
		data, err = cl.ProxyAction(ctx, *in)
	}
	if err != nil {
		out.Problem = problem(err, in != nil)
		return out
	}
	out.Data = &data
	return out
}
