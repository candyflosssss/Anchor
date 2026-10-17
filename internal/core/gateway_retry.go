package core

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

// A channel is one provider/key/model route. Its per-mapping retry count excludes
// the first attempt. All attempts and capped backoff share the provider timeout.
const channelRetryBackoff = 250 * time.Millisecond

type forwardAttemptResult struct {
	attempted bool
	success   bool
	retrySame bool
	status    int
	after     string
}

func (s *Server) forward(w http.ResponseWriter, r *http.Request, request map[string]json.RawMessage, public string, c state.Candidate, p state.Provider, key state.ProviderKey, stream bool, routeModel state.Model, affinity string) bool {
	retryLimit := routeModel.Settings.RetryLimit()
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(p.TimeoutSeconds)*time.Second)
	defer cancel()

	// Keep a half-open reservation for the entire retry group. Apply health
	// results afterwards so a circuit transition cannot preempt this retry group.
	attempts := make([]forwardAttemptResult, 0, retryLimit+1)
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
		if !next || !result.retrySame || retries >= retryLimit {
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

func channelRetryDelay(retry int) time.Duration {
	return channelRetryBackoff << min(max(retry, 0), 2)
}

func waitChannelRetry(ctx context.Context, retry int) error {
	// Additional configurable retries retain the 1s cap, avoiding duration overflow.
	timer := time.NewTimer(channelRetryDelay(retry))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}
