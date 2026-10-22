package desktop

import (
	"context"
	"time"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (b *Bridge) APIService() APIServiceResult { return b.apiServiceCall(nil) }
func (b *Bridge) SaveAPIService(in SaveAPIServiceSettings) APIServiceResult {
	if in.Validate() != nil {
		return APIServiceResult{Contract: Contract, Problem: &Problem{"invalid"}}
	}
	return b.apiServiceCall(func(ctx context.Context, c *client.Client) (APIServiceWorkspace, error) {
		return c.SaveAPIService(ctx, in)
	})
}
func (b *Bridge) RevokeAPIKey(in RevokeAPIKey) APIServiceResult {
	if in.ID == "" || len(in.ID) > 128 {
		return APIServiceResult{Contract: Contract, Problem: &Problem{"invalid"}}
	}
	return b.apiServiceCall(func(ctx context.Context, c *client.Client) (APIServiceWorkspace, error) {
		if _, err := c.RevokeAPIKey(ctx, in.ID); err != nil {
			return APIServiceWorkspace{}, err
		}
		return c.APIService(ctx)
	})
}
func (b *Bridge) apiServiceCall(write func(context.Context, *client.Client) (APIServiceWorkspace, error)) APIServiceResult {
	out := APIServiceResult{Contract: Contract}
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	c, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer c.Close()
	var data APIServiceWorkspace
	if write == nil {
		data, err = c.APIService(ctx)
	} else {
		data, err = write(ctx, c)
	}
	if err != nil {
		out.Problem = problem(err, write != nil)
		return out
	}
	out.Data = &data
	return out
}
func (b *Bridge) IssueAPIKey(in IssueAPIKey) IssuedAPIKeyResult {
	out := IssuedAPIKeyResult{Contract: Contract}
	if in.Validate() != nil {
		out.Problem = &Problem{"invalid"}
		return out
	}
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	c, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer c.Close()
	data, err := c.IssueAPIKey(ctx, state.IssueAPIKey(in))
	if err != nil {
		out.Problem = problem(err, true)
		return out
	}
	out.Data = &data
	return out
}
