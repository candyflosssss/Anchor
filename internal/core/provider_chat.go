package core

import (
	"context"
	"net/http"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

// Chat and probe runs share bounded lifecycle/cancellation, never their payloads.
// No prompt or generated text is written to state, request logs or system logs.
func (s *Server) providerChat(w http.ResponseWriter, r *http.Request) {
	var in catalog.ChatRequest
	if !decode(w, r, &in) {
		return
	}
	if in.ProviderID != r.PathValue("id") || in.Validate() != nil {
		apiError(w, state.ErrInvalid)
		return
	}
	if in.Action != "start" {
		s.mu.Lock()
		run := s.probeRuns[in.RunID]
		if run == nil || !run.chat || run.progress.ProviderID != in.ProviderID || run.progress.Revision != in.ExpectedRevision {
			s.mu.Unlock()
			apiError(w, state.ErrNotFound)
			return
		}
		if in.Action == "cancel" {
			run.cancel()
		}
		progress := run.progress
		if progress.Status == "running" {
			progress.ElapsedMS = time.Since(run.started).Milliseconds()
		}
		s.mu.Unlock()
		writeJSON(w, 200, progress)
		return
	}
	p, err := s.Store.Provider(in.ProviderID)
	if err != nil {
		apiError(w, err)
		return
	}
	if p.Revision != in.ExpectedRevision {
		apiError(w, state.ErrConflict)
		return
	}
	if !p.Enabled {
		apiError(w, state.ErrInvalid)
		return
	}
	keys, err := s.Store.ProviderKeys(p.ID)
	if err != nil {
		apiError(w, err)
		return
	}
	selected := state.ProviderKey{ID: "anonymous", Enabled: true, Protocol: p.Protocol}
	valid := len(keys) == 0 && in.KeyID == "anonymous"
	for _, key := range keys {
		if key.ID == in.KeyID && key.Enabled {
			valid = true
			selected = key
		}
	}
	if !valid {
		apiError(w, state.ErrInvalid)
		return
	}
	// Ensure the Key snapshot was read under the accepted provider revision.
	current, err := s.Store.Provider(p.ID)
	if err != nil {
		apiError(w, err)
		return
	}
	if current.Revision != p.Revision {
		apiError(w, state.ErrConflict)
		return
	}
	id, err := state.Token()
	if err != nil {
		apiError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), min(time.Duration(p.TimeoutSeconds)*time.Second, 120*time.Second))
	run := &probeRun{chat: true, progress: catalog.ProbeProgress{ProviderID: p.ID, Revision: p.Revision, RunID: id, KeyID: in.KeyID, Model: in.Model, Status: "running", Phase: "connecting"}, started: time.Now(), cancel: cancel, done: make(chan struct{})}
	s.mu.Lock()
	if s.probeRuns == nil {
		s.probeRuns = map[string]*probeRun{}
	}
	if len(s.probeRuns) >= 64 || s.stopping.Load() {
		s.mu.Unlock()
		cancel()
		fail(w, 503, "busy", "Too many tests")
		return
	}
	s.probeRuns[id] = run
	// Active runs live at most 120 seconds; completed text expires within 3 minutes.
	run.expiry = time.AfterFunc(3*time.Minute, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.probeRuns[id] == run {
			run.cancel()
			delete(s.probeRuns, id)
		}
	})
	initial := run.progress
	s.mu.Unlock()
	go func() {
		defer close(run.done)
		defer cancel()
		result, err := s.inferenceObserved(ctx, p, in.KeyID, in.Model, in.Messages, &selected, func(phase, text string) {
			s.mu.Lock()
			run.progress.Phase = phase
			run.progress.Text = text
			s.mu.Unlock()
		})
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			result = &catalog.Probe{Revision: p.Revision, KeyID: in.KeyID, Model: in.Model, At: run.started.Unix(), Failure: "invalid_response"}
		}
		run.progress.Status = "completed"
		run.progress.Phase = "complete"
		run.progress.Result = result
		run.progress.ElapsedMS = time.Since(run.started).Milliseconds()
		if ctx.Err() == context.Canceled {
			run.progress.Status = "cancelled"
		}
	}()
	writeJSON(w, 200, initial)
}
