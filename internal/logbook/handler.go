package logbook

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

var eventSequence atomic.Uint64

func NewEvent(level, source, event, requestID string, fields map[string]string) Event {
	return Event{ID: fmt.Sprintf("evt_%x_%x", time.Now().UnixNano(), eventSequence.Add(1)), AtMS: time.Now().UnixMilli(), Level: level, Source: source, Event: event, RequestID: requestID, Fields: fields}
}

// Handler persists only code-owned messages and a closed attribute allowlist.
// Arbitrary HTTP errors, upstream content and credentials never enter the log DB.
type Handler struct {
	next  slog.Handler
	store *Store
	attrs []slog.Attr
}

func NewHandler(next slog.Handler, s *Store) slog.Handler { return &Handler{next: next, store: s} }

var systemMessages = map[string]string{
	"core ready": "core.started", "core stopped": "core.stopped", "bindings preserved for restart": "core.restart_handoff", "binding startup": "binding.startup", "binding shutdown": "binding.shutdown",
	"agent startup reconciliation unavailable": "binding.startup_failed", "agent recovery incomplete; run anchor recover": "binding.recovery_failed",
	"drain deadline exceeded; remaining requests canceled": "core.drain_timeout", "usage persistence failed": "usage.write_failed", "test usage persistence failed": "usage.write_failed",
}

func (h *Handler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= slog.LevelInfo || h.next.Enabled(ctx, l)
}
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if event, ok := systemMessages[r.Message]; ok {
		fields := map[string]string{}
		add := func(a slog.Attr) bool {
			switch a.Key {
			case "instance_id", "binding_id", "status":
				v := a.Value.Resolve().String()
				if len(v) <= 128 {
					fields[a.Key] = v
				}
			}
			return true
		}
		for _, a := range h.attrs {
			add(a)
		}
		r.Attrs(add)
		level := "info"
		if r.Level >= slog.LevelError {
			level = "error"
		} else if r.Level >= slog.LevelWarn {
			level = "warn"
		}
		h.store.Event(NewEvent(level, "core", event, "", fields))
	}
	if h.next.Enabled(ctx, r.Level) {
		return h.next.Handle(ctx, r)
	}
	return nil
}
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{next: h.next.WithAttrs(attrs), store: h.store, attrs: append(append([]slog.Attr(nil), h.attrs...), attrs...)}
}
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name), store: h.store, attrs: append([]slog.Attr(nil), h.attrs...)}
}
