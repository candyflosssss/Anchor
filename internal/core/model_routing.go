package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

type modelSchedule struct {
	revision int64
	current  []float64
}
type routeAffinity struct {
	provider, model, key string
	revision             int64
	providerRevision     int64
	at                   time.Time
}
type routeLatency struct {
	ms float64
	at time.Time
}

func candidateAllowsKey(ids []string, key string) bool {
	if len(ids) == 0 {
		return true
	}
	for _, id := range ids {
		if id == key {
			return true
		}
	}
	return false
}

// Only a digest of the authenticated caller and stable request prefix is retained.
// It is a routing hint, not a promise that the upstream will report a cache hit.
func routingAffinity(owner, model string, request map[string]json.RawMessage) string {
	h := sha256.New()
	h.Write([]byte(owner))
	h.Write([]byte{0})
	h.Write([]byte(model))
	appendPart := func(raw []byte) {
		h.Write([]byte{0})
		if len(raw) > 65536 {
			raw = raw[:65536]
		}
		h.Write(raw)
	}
	appendPart(request["instructions"])
	var input []json.RawMessage
	if json.Unmarshal(request["input"], &input) == nil && len(input) > 0 {
		appendPart(input[0])
	} else {
		appendPart(request["input"])
	}
	return hex.EncodeToString(h.Sum(nil))
}
func latencyID(public string, revision int64, c state.Candidate, p state.Provider) string {
	raw, _ := json.Marshal([]any{public, revision, c.ProviderID, c.Model, p.Revision})
	return string(raw)
}
func (s *Server) routeCandidates(m state.Model) ([]state.Candidate, error) {
	return s.routeCandidatesFor(m, "")
}

// Availability is checked without reserving half-open Keys. The selected Key and
// concurrency are rechecked immediately before forwarding by the existing gates.
func (s *Server) routeCandidatesFor(m state.Model, affinity string) ([]state.Candidate, error) {
	if m.RoutingStrategy() == "priority" {
		return m.Candidates, nil
	}
	providers := make([]state.Provider, len(m.Candidates))
	keys := make([][]state.ProviderKey, len(m.Candidates))
	for i, c := range m.Candidates {
		p, err := s.Store.Provider(c.ProviderID)
		if err != nil {
			return nil, err
		}
		providers[i] = p
		keys[i], err = s.Store.ProviderKeys(p.ID)
		if err != nil {
			return nil, err
		}
		if len(keys[i]) == 0 {
			keys[i] = []state.ProviderKey{{ID: "anonymous", Enabled: true}}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	available := make([]int, 0, len(m.Candidates))
	for i, c := range m.Candidates {
		p := providers[i]
		if !p.Enabled || s.active[p.ID] >= p.MaxConcurrent {
			continue
		}
		for _, k := range keys[i] {
			h := s.health[healthID(p, k.ID)]
			if k.Enabled && candidateAllowsKey(c.KeyIDs, k.ID) && (h == nil || (!h.Invalid && !h.Probing && !h.Until.After(now))) {
				available = append(available, i)
				break
			}
		}
	}
	if len(available) == 0 {
		return []state.Candidate{}, nil
	}
	strategy := m.RoutingStrategy()
	if strategy == "lowest-price" {
		sort.SliceStable(available, func(i, j int) bool {
			return m.Candidates[available[i]].EffectiveMultiplier() < m.Candidates[available[j]].EffectiveMultiplier()
		})
	} else if strategy == "lowest-latency" {
		score := func(i int) (float64, bool) {
			v, ok := s.latency[latencyID(m.ID, m.Revision, m.Candidates[i], providers[i])]
			return v.ms, ok && now.Sub(v.at) < 30*time.Minute
		}
		sort.SliceStable(available, func(i, j int) bool {
			a, ka := score(available[i])
			b, kb := score(available[j])
			if ka != kb {
				return !ka
			}
			return ka && a < b
		})
	} else {
		preferred := -1
		if strategy == "cache-priority" {
			if a, ok := s.affinity[affinity]; ok && a.revision == m.Revision && now.Sub(a.at) < 10*time.Minute {
				for _, i := range available {
					c := m.Candidates[i]
					if c.ProviderID == a.provider && c.Model == a.model && a.providerRevision == providers[i].Revision && candidateAllowsKey(c.KeyIDs, a.key) {
						for _, k := range keys[i] {
							h := s.health[healthID(providers[i], k.ID)]
							if k.ID == a.key && k.Enabled && (h == nil || (!h.Invalid && !h.Probing && !h.Until.After(now))) {
								preferred = i
								break
							}
						}
					}
				}
			}
		}
		if preferred < 0 {
			schedule := s.routing[m.ID]
			if schedule == nil || schedule.revision != m.Revision || len(schedule.current) != len(m.Candidates) {
				schedule = &modelSchedule{revision: m.Revision, current: make([]float64, len(m.Candidates))}
				s.routing[m.ID] = schedule
			}
			usable := map[int]bool{}
			for _, i := range available {
				usable[i] = true
			}
			total := 0.0
			for i, c := range m.Candidates {
				if !usable[i] {
					schedule.current[i] = 0
					continue
				}
				weight := 1 / c.EffectiveMultiplier()
				total += weight
				schedule.current[i] += weight
				if preferred < 0 || schedule.current[i] > schedule.current[preferred] {
					preferred = i
				}
			}
			schedule.current[preferred] -= total
		}
		for pos, i := range available {
			if i == preferred {
				available = append([]int{i}, append(available[:pos], available[pos+1:]...)...)
				break
			}
		}
	}
	out := make([]state.Candidate, 0, len(available))
	for _, i := range available {
		out = append(out, m.Candidates[i])
	}
	return out, nil
}
func (s *Server) preferredRouteKey(m state.Model, c state.Candidate, p state.Provider, affinity string) string {
	if m.RoutingStrategy() != "cache-priority" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.affinity[affinity]
	if ok && a.revision == m.Revision && a.provider == c.ProviderID && a.model == c.Model && a.providerRevision == p.Revision && time.Since(a.at) < 10*time.Minute && candidateAllowsKey(c.KeyIDs, a.key) {
		return a.key
	}
	return ""
}
func (s *Server) rememberRoute(m state.Model, c state.Candidate, p state.Provider, key, affinity string, elapsed time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	id := latencyID(m.ID, m.Revision, c, p)
	v, ok := s.latency[id]
	ms := float64(elapsed.Microseconds()) / 1000
	if ok && now.Sub(v.at) < 30*time.Minute {
		ms = v.ms*0.8 + ms*0.2
	}
	if len(s.latency) >= 4096 {
		for k, v := range s.latency {
			if now.Sub(v.at) >= 30*time.Minute {
				delete(s.latency, k)
			}
		}
		if len(s.latency) >= 4096 {
			oldest := ""
			for k, v := range s.latency {
				if oldest == "" || v.at.Before(s.latency[oldest].at) {
					oldest = k
				}
			}
			delete(s.latency, oldest)
		}
	}
	s.latency[id] = routeLatency{ms: ms, at: now}
	if m.RoutingStrategy() != "cache-priority" || affinity == "" {
		return
	}
	if len(s.affinity) >= 1024 {
		for k, v := range s.affinity {
			if now.Sub(v.at) >= 10*time.Minute {
				delete(s.affinity, k)
			}
		}
		if len(s.affinity) >= 1024 {
			oldest := ""
			for k, v := range s.affinity {
				if oldest == "" || v.at.Before(s.affinity[oldest].at) {
					oldest = k
				}
			}
			delete(s.affinity, oldest)
		}
	}
	s.affinity[affinity] = routeAffinity{provider: c.ProviderID, model: c.Model, key: key, revision: m.Revision, providerRevision: p.Revision, at: now}
}
