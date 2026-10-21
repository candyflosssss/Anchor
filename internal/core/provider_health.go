package core

import (
	"encoding/json"
	"fmt"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type keyHealth struct {
	Failures int
	Score    int
	errors   []keyError
	Until    time.Time
	Test     *catalog.Probe
}
type keyError struct {
	score int
	until time.Time
}

func (h *keyHealth) expire(now time.Time) {
	if !h.Until.IsZero() && !h.Until.After(now) {
		h.Until = time.Time{}
		h.errors = nil
		h.Failures = 0
	}
	h.Score = 0
	retained := h.errors[:0]
	for _, e := range h.errors {
		if e.until.After(now) {
			retained = append(retained, e)
			h.Score += e.score
		}
	}
	h.errors = retained
}

// Remove obsolete revisions while preserving newer snapshots used by in-flight requests.
func (s *Server) pruneHealthLocked(p state.Provider) {
	prefix := p.ID + "/"
	for id := range s.health {
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		rest := strings.TrimPrefix(id, prefix)
		rev, _, _ := strings.Cut(rest, "/")
		n, err := strconv.ParseInt(rev, 10, 64)
		if err == nil && n < p.Revision {
			delete(s.health, id)
		}
	}
}
func healthID(p state.Provider, k string) string { return fmt.Sprintf("%s/%d/%s", p.ID, p.Revision, k) }
func (s *Server) chooseKey(p state.Provider, tried map[string]bool) (state.ProviderKey, bool) {
	return s.chooseRouteKey(p, tried, nil, "")
}
func (s *Server) chooseRouteKey(p state.Provider, tried map[string]bool, allowed []string, preferred string) (state.ProviderKey, bool) {
	keys, err := s.Store.ProviderKeys(p.ID)
	if err != nil {
		return state.ProviderKey{}, false
	}
	if len(keys) == 0 {
		keys = []state.ProviderKey{{ID: "anonymous", Enabled: true, Protocol: p.Protocol}}
	}
	strategy := s.Store.KeyStrategy(p.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneHealthLocked(p)
	eligible := []state.ProviderKey{}
	weights := []float64{}
	now := time.Now()
	maximum := 0.0
	for _, k := range keys {
		if !k.Enabled || tried[k.ID] || !candidateAllowsKey(allowed, k.ID) {
			continue
		}
		weight := s.keyWeightLocked(p, k.ID, now)
		if weight == 0 {
			continue
		}
		eligible = append(eligible, k)
		weights = append(weights, weight)
		maximum = max(maximum, weight)
	}
	if len(eligible) == 0 {
		return state.ProviderKey{}, false
	}
	for i, k := range eligible {
		if k.ID == preferred && weights[i] == maximum {
			return k, true
		}
	}
	if strategy == "priority" {
		return eligible[0], true
	} else {
		raw, _ := json.Marshal([]any{p.Revision, eligible, weights})
		scheduleID := "keys\x00" + p.ID
		schedule := s.routing[scheduleID]
		if schedule == nil || schedule.signature != string(raw) {
			schedule = &modelSchedule{signature: string(raw), current: make([]float64, len(eligible))}
			s.routing[scheduleID] = schedule
		}
		best, total := 0, 0.0
		for i, weight := range weights {
			total += weight
			schedule.current[i] += weight
			if schedule.current[i] > schedule.current[best] {
				best = i
			}
		}
		schedule.current[best] -= total
		return eligible[best], true
	}
	return state.ProviderKey{}, false
}
func configuredRetryDelay(v string, fallback int) time.Duration {
	d := time.Duration(fallback) * time.Second
	if n, e := strconv.ParseInt(v, 10, 64); e == nil && n >= 0 {
		if n > 3600 {
			n = 3600
		}
		d = time.Duration(n) * time.Second
	} else if t, e := http.ParseTime(v); e == nil {
		d = time.Until(t)
	}
	if d < time.Second {
		d = time.Second
	}
	if d > time.Hour {
		d = time.Hour
	}
	return d
}
func (s *Server) finishKey(p state.Provider, k string, status int, after string, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneHealthLocked(p)
	id := healthID(p, k)
	h := s.health[id]
	if h == nil {
		h = &keyHealth{}
		s.health[id] = h
	}
	now := time.Now()
	h.expire(now)
	policy := s.keyPolicy
	// In-flight successes/failures cannot reopen or extend a cooling Key.
	if success || !policy.Enabled || h.Until.After(now) {
		return
	}
	score, window := 0, policy.ErrorWindowSeconds
	switch {
	case status == 401 || status == 403:
		score = 100
	case status == 429:
		score, window = 50, min(window, 60)
	case status == 0 || status >= 500 || status == 408:
		score, window = 25, min(window, 60)
	}
	if score == 0 {
		return
	}
	h.Failures++
	h.errors = append(h.errors, keyError{score: score, until: now.Add(time.Duration(window) * time.Second)})
	h.Score += score
	if h.Score >= policy.FailureThreshold {
		delay := time.Duration(policy.CooldownSeconds) * time.Second
		if status == 429 {
			delay = max(delay, configuredRetryDelay(after, policy.RateLimitSeconds))
		}
		h.Until = now.Add(delay)
	}
}
func (s *Server) keyStatus(p state.Provider, k state.ProviderKey) catalog.KeyStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := catalog.KeyStatus{Rate: k.Rate, EffectiveMultiplier: p.EffectiveKeyMultiplier(k), ID: k.ID, Enabled: k.Enabled, State: "ready", Protocol: k.Protocol, ModelProtocols: k.ModelProtocols}
	if !k.Enabled {
		out.State = "disabled"
	}
	if h := s.health[healthID(p, k.ID)]; h != nil {
		h.expire(time.Now())
		out.Failures = h.Failures
		out.ErrorScore = h.Score
		out.Test = h.Test
		if s.keyWeightLocked(p, k.ID, time.Now()) < 1 {
			out.State = "cooling"
			out.RetryAt = h.Until.Unix()
		}
	}
	if !k.Enabled {
		out.State = "disabled"
	}
	return out
}

// keyWeightLocked is a scheduling factor, independent of billing multipliers.
// Call with s.mu held. Expiry has no exclusive probe or request reservation.
func (s *Server) keyWeightLocked(p state.Provider, key string, now time.Time) float64 {
	if h := s.health[healthID(p, key)]; h != nil {
		h.expire(now)
		if s.keyPolicy.Enabled && h.Until.After(now) {
			return 0
		}
	}
	return 1
}

// pickURL rotates across a provider's request base URLs for load balancing.
// Discovery and connectivity tests keep using the first URL for determinism.
func (s *Server) pickURL(p state.Provider) string {
	urls := p.URLs()
	if len(urls) == 0 {
		return p.BaseURL
	}
	if len(urls) == 1 {
		return urls[0]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := "url\x00" + p.ID
	i := s.cursor[key] % len(urls)
	s.cursor[key] = (i + 1) % len(urls)
	return urls[i]
}
