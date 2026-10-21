package desktop

import (
	"context"
	"time"
)

func (b *Bridge) ImportCCSwitch(in CCImportRequest) CCImportResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := CCImportResult{Contract: Contract}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	data, err := cl.ImportCCSwitch(ctx, in)
	if err != nil {
		out.Problem = problem(err, in.Approved)
		return out
	}
	out.Data = &data
	return out
}
