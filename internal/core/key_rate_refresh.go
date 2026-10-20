package core

import (
	"context"
	"sync"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/providerapi"
)

type keyRateRefresh struct {
	mu     sync.Mutex
	wake   chan struct{}
	done   chan struct{}
	cancel context.CancelFunc
}

func newKeyRateRefresh() *keyRateRefresh {
	return &keyRateRefresh{wake: make(chan struct{}, 1)}
}

func (f *keyRateRefresh) start(s *Server) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.done != nil || f.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	f.done = make(chan struct{})
	go func() {
		defer close(f.done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		attempts := map[rateTarget]rateAttempt{}
		for {
			s.refreshKeyRates(ctx, time.Now(), attempts)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-f.wake:
			}
		}
	}()
}

func (f *keyRateRefresh) notify() {
	select {
	case f.wake <- struct{}{}:
	default:
	}
}

func (f *keyRateRefresh) close() {
	f.mu.Lock()
	if f.cancel == nil {
		// Prevent a later start even when Close precedes Run.
		f.cancel = func() {}
	}
	f.cancel()
	done := f.done
	f.mu.Unlock()
	if done != nil {
		<-done
	}
}

type rateTarget struct{ provider, key string }
type rateAttempt struct {
	revision int64
	at       time.Time
}

func keyRateDue(rate *catalog.KeyRateCache, now time.Time) bool {
	if rate == nil {
		return true
	}
	if rate.Failure == "" && rate.UsableAt(now) {
		observed, _ := time.Parse(time.RFC3339Nano, rate.ObservedAt)
		if now.Sub(observed) < catalog.KeyRateRefreshInterval {
			return false
		}
	}
	attempted, err := time.Parse(time.RFC3339Nano, rate.AttemptedAt)
	return err != nil || now.Sub(attempted) >= time.Minute || now.Before(attempted)
}

// One worker serializes all maintenance reads, with no inference probes. The
// shared query entry enforces provider/Key identity, egress, revision checks,
// eight-second deadlines, and existing concurrency limits. Busy sources wait
// for the next minute instead of delaying inference or hammering on mutations.
func (s *Server) refreshKeyRates(ctx context.Context, now time.Time, attempts map[rateTarget]rateAttempt) {
	providers, err := s.Store.Providers()
	if err != nil {
		return
	}
	live := map[rateTarget]bool{}
	for _, p := range providers {
		if ctx.Err() != nil {
			return
		}
		if !p.Enabled {
			continue
		}
		keys, err := s.Store.ProviderKeys(p.ID)
		if err != nil {
			continue
		}
		for _, key := range keys {
			if ctx.Err() != nil {
				return
			}
			if !key.Enabled {
				continue
			}
			target := rateTarget{p.ID, key.ID}
			live[target] = true
			previous, seen := attempts[target]
			if seen && previous.revision == p.Revision && now.Sub(previous.at) < time.Minute {
				continue
			}
			if !keyRateDue(key.Rate, now) {
				continue
			}
			attempts[target] = rateAttempt{p.Revision, now}
			_, _ = s.QueryProvider(ctx, providerapi.Query{ProviderID: p.ID, ExpectedRevision: p.Revision, KeyID: key.ID, Operation: "effective-rate"})
		}
	}
	for target := range attempts {
		if !live[target] {
			delete(attempts, target)
		}
	}
}
