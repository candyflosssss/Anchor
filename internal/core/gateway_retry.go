package core

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

// A channel is one provider/key/model route. These are retries after the first
// attempt, not the total attempt count. All attempts share the provider timeout.
const defaultChannelRetries = 3

const channelRetryBackoff = 250 * time.Millisecond

type forwardAttemptResult struct {
	attempted bool
	success   bool
	retrySame bool
	status    int
	after     string
}

func (s *Server) forward(w http.ResponseWriter, r *http.Request, request map[string]json.RawMessage, public string, c state.Candidate, p state.Provider, key state.ProviderKey, stream bool, routeModel state.Model, affinity string) bool {
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(p.TimeoutSeconds)*time.Second)
	defer cancel()

	// Keep a half-open reservation for the entire retry group. Apply health
	// results afterwards so the third failed attempt cannot preempt retry #3.
	attempts := make([]forwardAttemptResult, 0, defaultChannelRetries+1)
	defer func() {
		if len(attempts) == 0 || r.Context().Err() != nil {
			s.mu.Lock()
			if h := s.health[healthID(p, key.ID)]; h != nil {
				h.Probing = false
			}
			s.mu.Unlock()
			return
		}
		for _, result := range attempts {
			s.finishKey(p, key.ID, result.status, result.after, result.success)
		}
	}()

	for retries := 0; ; retries++ {
		var result forwardAttemptResult
		next := s.forwardAttempt(w, r, request, public, c, p, key, stream, routeModel, affinity, ctx, &result)
		if result.attempted {
			attempts = append(attempts, result)
		}
		if next && ctx.Err() != nil {
			if r.Context().Err() == nil {
				fail(w, http.StatusGatewayTimeout, "upstream_timeout", "upstream retry budget timed out")
			}
			return false
		}
		if !next || !result.retrySame || retries == defaultChannelRetries {
			return next
		}
		if err := waitChannelRetry(ctx, retries); err != nil {
			if r.Context().Err() == nil {
				fail(w, http.StatusGatewayTimeout, "upstream_timeout", "upstream retry budget timed out")
			}
			return false
		}
	}
}

func waitChannelRetry(ctx context.Context, retry int) error {
	timer := time.NewTimer(channelRetryBackoff << retry)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}
