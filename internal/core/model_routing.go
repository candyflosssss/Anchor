package core

import (
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

type modelSchedule struct {
	revision int64
	current  []float64
}

// routeCandidates uses inverse cost multipliers for smooth weighted round robin.
// Availability checks do not reserve a half-open Key; chooseKey remains the sole
// owner of that reservation, and take rechecks concurrency before forwarding.
func (s *Server) routeCandidates(m state.Model) ([]state.Candidate, error) {
	if m.RoutingStrategy() == "priority" || len(m.Candidates) < 2 {
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
	schedule := s.routing[m.ID]
	if schedule == nil || schedule.revision != m.Revision || len(schedule.current) != len(m.Candidates) {
		schedule = &modelSchedule{revision: m.Revision, current: make([]float64, len(m.Candidates))}
		s.routing[m.ID] = schedule
	}
	available := make([]bool, len(m.Candidates))
	winner := -1
	total := 0.0
	now := time.Now()
	for i, c := range m.Candidates {
		p := providers[i]
		if p.Enabled && s.active[p.ID] < p.MaxConcurrent {
			for _, k := range keys[i] {
				h := s.health[healthID(p, k.ID)]
				if k.Enabled && (h == nil || (!h.Invalid && !h.Probing && !h.Until.After(now))) {
					available[i] = true
					break
				}
			}
		}
		if !available[i] {
			schedule.current[i] = 0
			continue
		}
		weight := 1 / c.EffectiveMultiplier()
		total += weight
		schedule.current[i] += weight
		if winner < 0 || schedule.current[i] > schedule.current[winner] {
			winner = i
		}
	}
	out := make([]state.Candidate, 0, len(m.Candidates))
	if winner < 0 {
		return out, nil
	}
	schedule.current[winner] -= total
	out = append(out, m.Candidates[winner])
	// Fallback never replays the selected candidate; existing retry restrictions apply.
	for i, c := range m.Candidates {
		if available[i] && i != winner {
			out = append(out, c)
		}
	}
	return out, nil
}
