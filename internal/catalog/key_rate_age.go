package catalog

import "time"

const KeyRateRefreshInterval = time.Hour
const KeyRateMaxAge = 3 * time.Hour

// UsableAt keeps the last observation visible while preventing an expired or
// undated multiplier from entering new estimates and routing decisions.
func (r *KeyRateCache) UsableAt(now time.Time) bool {
	if r == nil || r.Multiplier == nil {
		return false
	}
	observed, err := time.Parse(time.RFC3339Nano, r.ObservedAt)
	return err == nil && now.Sub(observed) >= 0 && now.Sub(observed) < KeyRateMaxAge
}
