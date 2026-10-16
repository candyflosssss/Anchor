package desktop

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

func (b *Bridge) LogSuggestions(q LogSuggestionsQuery) LogSuggestionsResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := LogSuggestionsResult{Contract: Contract}
	if q.Validate() != nil {
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
	v, err := cl.LogSuggestions(ctx, q)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	out.Data = &v
	return out
}

func (b *Bridge) LogStats(q LogStatsQuery) LogStatsResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := LogStatsResult{Contract: Contract}
	if q.Validate() != nil {
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
	v, err := cl.LogStats(ctx, q)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	out.Data = &v
	return out
}
func (b *Bridge) Logs(q LogQuery) LogsResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := LogsResult{Contract: Contract}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	v, err := cl.Logs(ctx, q)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	out.Data = &v
	return out
}
func (b *Bridge) LogDetail(id string) LogDetailResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := LogDetailResult{Contract: Contract}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	v, err := cl.LogDetail(ctx, id)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	out.Data = &v
	return out
}
func (b *Bridge) LogSettings() LogSettingsResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := LogSettingsResult{Contract: Contract}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	cl, _, err := s.connection(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	v, err := cl.LogSettings(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	out.Data = &v
	return out
}
func (b *Bridge) ConfigureLogs(in LogSettings) LogSettingsResult {
	s := b.session
	s.mu.Lock()
	defer s.mu.Unlock()
	out := LogSettingsResult{Contract: Contract}
	if in.Validate() != nil {
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
	v, err := cl.ConfigureLogs(ctx, in)
	if err != nil {
		out.Problem = problem(err, true)
		return out
	}
	out.Data = &v
	return out
}

// The path comes only from a native save dialog; JavaScript cannot supply it.
func (b *Bridge) ExportLogs(q LogQuery) StopResult {
	s := b.session
	out := StopResult{Contract: Contract}
	if q.Validate() != nil {
		out.Problem = &Problem{"invalid"}
		return out
	}
	if s.options.SaveLogFile == nil {
		out.Problem = &Problem{"unsupported"}
		return out
	}
	path, err := s.options.SaveLogFile()
	if err != nil {
		out.Problem = &Problem{"read_error"}
		return out
	}
	if path == "" {
		out.Data = &StopInfo{Accepted: false}
		return out
	}
	s.mu.Lock()
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
	defer cancel()
	cl, _, err := s.connection(ctx)
	s.mu.Unlock()
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	f, err := os.CreateTemp(filepath.Dir(path), ".anchor-logs-*.partial")
	if err != nil {
		out.Problem = &Problem{"permission"}
		return out
	}
	defer os.Remove(f.Name())
	err = cl.ExportLogs(ctx, q, f)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		out.Problem = &Problem{"read_error"}
		return out
	}
	out.Data = &StopInfo{Accepted: true}
	return out
}
