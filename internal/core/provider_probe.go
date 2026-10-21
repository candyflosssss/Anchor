package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/candyflosssss/Anchor/internal/apierror"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

type probeRun struct {
	chat     bool
	progress catalog.ProbeProgress
	started  time.Time
	cancel   context.CancelFunc
	done     chan struct{}
	expiry   *time.Timer
}

func (s *Server) providerProbe(w http.ResponseWriter, r *http.Request) {
	var in catalog.ProbeRequest
	if !decode(w, r, &in) {
		return
	}
	if in.ProviderID != r.PathValue("id") || !state.ValidProviderID(in.ProviderID) || in.ExpectedRevision < 1 {
		apiError(w, state.ErrInvalid)
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
	if in.Action == "start" {
		if !in.Approved || in.Model == "" || len(in.Model) > 256 || strings.ContainsAny(in.Model, "\r\n") {
			apiError(w, state.ErrInvalid)
			return
		}
		keys, e := s.Store.ProviderKeys(p.ID)
		if e != nil {
			apiError(w, e)
			return
		}
		valid := len(keys) == 0 && in.KeyID == "anonymous"
		for _, k := range keys {
			if k.ID == in.KeyID && k.Enabled {
				valid = true
			}
		}
		if !valid {
			apiError(w, state.ErrInvalid)
			return
		}
		id, e := state.Token()
		if e != nil {
			apiError(w, e)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		run := &probeRun{progress: catalog.ProbeProgress{ProviderID: p.ID, Revision: p.Revision, RunID: id, KeyID: in.KeyID, Model: in.Model, Status: "running", Phase: "connecting"}, started: time.Now(), cancel: cancel, done: make(chan struct{})}
		s.mu.Lock()
		if s.probeRuns == nil {
			s.probeRuns = map[string]*probeRun{}
		}
		for token, old := range s.probeRuns {
			if !old.chat && time.Since(old.started) > time.Minute {
				old.cancel()
				delete(s.probeRuns, token)
			}
		}
		if len(s.probeRuns) >= 64 || s.stopping.Load() {
			s.mu.Unlock()
			cancel()
			fail(w, apierror.Busy)
			return
		}
		s.probeRuns[id] = run
		run.expiry = time.AfterFunc(time.Minute, func() {
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
			firstByteMS := int64(0)
			result, e := s.probeObserved(ctx, p, in.KeyID, in.Model, func(phase, text string) {
				s.mu.Lock()
				if phase == "receiving" && text != "" && run.progress.FirstByteMS == 0 {
					firstByteMS = max(1, time.Since(run.started).Milliseconds())
					run.progress.FirstByteMS = firstByteMS
				}
				run.progress.Phase = phase
				run.progress.Text = text
				s.mu.Unlock()
			})
			s.mu.Lock()
			defer s.mu.Unlock()
			if e != nil {
				result = &catalog.Probe{Revision: p.Revision, KeyID: in.KeyID, Model: in.Model, At: run.started.Unix(), Failure: "invalid_response"}
			}
			run.progress.Status = "completed"
			run.progress.Phase = "complete"
			run.progress.Result = result
			run.progress.ElapsedMS = max(firstByteMS, time.Since(run.started).Milliseconds())
			run.progress.FirstByteMS = firstByteMS
			if errors.Is(ctx.Err(), context.Canceled) {
				run.progress.Status = "cancelled"
			}
		}()
		writeJSON(w, 200, initial)
		return
	}
	if in.Action != "read" && in.Action != "cancel" {
		apiError(w, state.ErrInvalid)
		return
	}
	s.mu.Lock()
	run := s.probeRuns[in.RunID]
	if run == nil || run.chat || run.progress.ProviderID != p.ID || run.progress.Revision != p.Revision {
		s.mu.Unlock()
		apiError(w, state.ErrNotFound)
		return
	}
	if in.Action == "cancel" {
		run.cancel()
	}
	progress := run.progress
	if progress.Status == "running" {
		progress.ElapsedMS = max(progress.FirstByteMS, time.Since(run.started).Milliseconds())
	}
	s.mu.Unlock()
	writeJSON(w, 200, progress)
}

// probeOutput consumes normalized SSE produced by the existing protocol converter.
// Only output text is exposed; raw events, credentials and upstream errors are excluded.
type probeOutput struct {
	header  http.Header
	pending string
	text    string
	secret  string
	limit   int
	strict  bool
	notify  func(string, string)
}

func (w *probeOutput) Header() http.Header { return w.header }
func (w *probeOutput) WriteHeader(int)     {}
func (w *probeOutput) Flush()              {}
func (w *probeOutput) Write(b []byte) (int, error) {
	w.pending += string(b)
	if len(w.pending) > 2<<20 {
		return 0, errors.New("probe event too large")
	}
	for {
		line, rest, ok := strings.Cut(w.pending, "\n")
		if !ok {
			break
		}
		w.pending = rest
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		}
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil || event.Type != "response.output_text.delta" {
			continue
		}
		limit := w.limit
		if limit == 0 {
			limit = 16384
		}
		if len(w.text)+len(event.Delta) > limit {
			return 0, errors.New("probe output too large")
		}
		w.text += event.Delta
		text := publicProbeText(w.text, w.secret, true)
		if w.strict && len(text) > limit {
			return 0, errors.New("probe public output too large")
		}
		w.notify("receiving", text)
	}
	return len(b), nil
}

// Hold a credential prefix across deltas so split echoes cannot leak fragments.
func publicProbeText(text, secret string, partial bool) string {
	if secret == "" {
		return text
	}
	text = strings.ReplaceAll(text, secret, "[redacted]")
	if partial {
		for n := min(len(secret)-1, len(text)); n > 0; n-- {
			if strings.HasSuffix(text, secret[:n]) {
				return text[:len(text)-n]
			}
		}
	}
	return text
}
