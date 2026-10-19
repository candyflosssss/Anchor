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
	signature string
	revision  int64
	current   []float64
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

// Health changes scheduling weight without excluding Keys. Manual availability
// and concurrency are rechecked immediately before forwarding.
func (s *Server) routeCandidatesFor(m state.Model, affinity string) ([]state.Candidate, error) {
	// Cost-based rules schedule actual provider/model/Key routes. The stored
	// candidate multiplier is reserved and intentionally never used here.
	strategy := m.RoutingStrategy()
	costs := []*float64{}
	routes := []state.Candidate{}
	providers := []state.Provider{}
	keys := [][]state.ProviderKey{}
	for _, c := range m.Candidates {
		p, err := s.Store.Provider(c.ProviderID)
		if err != nil {
			return nil, err
		}
		sourceKeys, err := s.Store.ProviderKeys(p.ID)
		if err != nil {
			return nil, err
		}
		if len(sourceKeys) == 0 {
			sourceKeys = []state.ProviderKey{{ID: "anonymous", Enabled: true}}
		}
		if strategy == "lowest-latency" || strategy == "priority" {
			routes = append(routes, c)
			providers = append(providers, p)
			keys = append(keys, sourceKeys)
			continue
		}
		for _, k := range sourceKeys {
			if !k.Enabled || !candidateAllowsKey(c.KeyIDs, k.ID) {
				continue
			}
			route := c
			route.KeyIDs = []string{k.ID}
			routes = append(routes, route)
			providers = append(providers, p)
			keys = append(keys, []state.ProviderKey{k})
			costs = append(costs, p.EffectiveKeyMultiplier(k))
		}
	}
	m.Candidates = routes
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	available := make([]int, 0, len(m.Candidates))
	healthWeights := make([]float64, len(m.Candidates))
	for i, c := range m.Candidates {
		p := providers[i]
		if !p.Enabled || s.active[p.ID] >= p.MaxConcurrent {
			continue
		}
		for _, k := range keys[i] {
			if k.Enabled && candidateAllowsKey(c.KeyIDs, k.ID) {
				healthWeights[i] = max(healthWeights[i], s.keyWeightLocked(p, k.ID, now))
			}
		}
		if healthWeights[i] > 0 {
			available = append(available, i)
		}
	}
	if len(available) == 0 {
		return []state.Candidate{}, nil
	}
	ordered := strategy == "priority" || strategy == "lowest-price" || strategy == "lowest-latency"
	if strategy == "priority" {
		// Retain configured order before applying health preference.
	} else if strategy == "lowest-price" {
		sort.SliceStable(available, func(i, j int) bool {
			a, b := costs[available[i]], costs[available[j]]
			if a == nil || b == nil {
				return a != nil && b == nil
			}
			return *a < *b
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
							if k.ID == a.key && k.Enabled {
								preferred = i
								break
							}
						}
					}
				}
			}
		}
		ordered = preferred >= 0
		if preferred < 0 {
			// Rebuild smooth weights after cache/config/Key changes, even when the
			// public model revision and endpoint count have not changed.
			signatureBytes, _ := json.Marshal([]any{m.Candidates, providers, costs, healthWeights})
			signature := string(signatureBytes)
			schedule := s.routing[m.ID]
			if schedule == nil || schedule.revision != m.Revision || schedule.signature != signature || len(schedule.current) != len(m.Candidates) {
				schedule = &modelSchedule{signature: signature, revision: m.Revision, current: make([]float64, len(m.Candidates))}
				s.routing[m.ID] = schedule
			}
			usable := map[int]bool{}
			for _, i := range available {
				usable[i] = true
			}
			// Known/free rates retain preference among the healthiest routes.
			// A degraded free/known route cannot suppress healthier alternatives.
			known, free := false, false
			maximumHealth := 0.0
			for _, i := range available {
				maximumHealth = max(maximumHealth, healthWeights[i])
			}
			minimum := 0.0
			for _, i := range available {
				if v := costs[i]; v != nil {
					if healthWeights[i] == maximumHealth {
						known = true
						if *v == 0 {
							free = true
						}
					}
					if *v > 0 && (minimum == 0 || *v < minimum) {
						minimum = *v
					}
				}
			}
			sort.SliceStable(available, func(a, b int) bool { return costs[available[a]] != nil && costs[available[b]] == nil })
			total := 0.0
			for i := range m.Candidates {
				if !usable[i] {
					schedule.current[i] = 0
					continue
				}
				weight := 1.0
				if known {
					v := costs[i]
					if v == nil || (free && *v != 0) {
						schedule.current[i] = 0
						continue
					}
					if !free && *v > 0 {
						weight = minimum / *v
					}
				}
				weight *= healthWeights[i]
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
	if ordered {
		// Ordered strategies retain their normal preference for healthy routes.
		// A degraded first choice periodically leads, otherwise moves behind
		// healthier alternatives; it always remains in the fallback list.
		raw, _ := json.Marshal([]any{m.Revision, m.Candidates, providers, healthWeights})
		id := "health\x00" + m.ID
		schedule := s.routing[id]
		if schedule == nil || schedule.signature != string(raw) {
			schedule = &modelSchedule{signature: string(raw), current: make([]float64, len(m.Candidates))}
			s.routing[id] = schedule
		}
		maximum := 0.0
		for _, i := range available {
			maximum = max(maximum, healthWeights[i])
		}
		for pos, i := range available {
			if healthWeights[i] < maximum {
				schedule.current[i] += healthWeights[i] / maximum
				if schedule.current[i] < 1-1e-9 {
					continue
				}
				schedule.current[i] -= 1
			}
			available = append([]int{i}, append(available[:pos], available[pos+1:]...)...)
			break
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
