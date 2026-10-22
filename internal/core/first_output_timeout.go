package core

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errFirstOutputTimeout = errors.New("first generated output timeout")

// Own the timer in the network layer. Protocol parsers only report semantic
// progress; they never control deadlines or close the caller's network reader.
type firstOutputTimeout struct {
	mu     sync.Mutex
	timer  *time.Timer
	cancel context.CancelCauseFunc
	done   bool
}

func withFirstOutputTimeout(parent context.Context, wait time.Duration) (context.Context, *firstOutputTimeout) {
	ctx, cancel := context.WithCancelCause(parent)
	f := &firstOutputTimeout{cancel: cancel}
	f.timer = time.AfterFunc(wait, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.done {
			f.done = true
			cancel(errFirstOutputTimeout)
		}
	})
	return ctx, f
}

func (f *firstOutputTimeout) observe() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.done {
		f.done = true
		f.timer.Stop()
	}
}
func (f *firstOutputTimeout) close() { f.observe(); f.cancel(nil) }
