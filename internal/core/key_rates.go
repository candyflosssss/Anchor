package core

import (
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/providerapi"
	"github.com/candyflosssss/Anchor/internal/state"
)

// Persist only sanitized, automatically identity-matched local Key observations.
// Explicit remote numeric IDs may refer to another Key and must never be cached.
func (s *Server) cacheKeyRate(p state.Provider, keys []state.ProviderKey, keyID string, result *providerapi.Result) error {
	rate := catalog.KeyRateCache{Failure: result.Failure}
	cfg, err := s.Store.ProviderPlatform(p.ID)
	if err != nil {
		return err
	}
	if v := result.EffectiveRate; v != nil {
		valid := len(v.Group) <= 512 && len(v.GroupName) <= 512 && !strings.ContainsFunc(v.Group+v.GroupName, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) })
		if cfg.UserToken != "" && strings.Contains(v.Group+v.GroupName, cfg.UserToken) {
			valid = false
		}
		for _, k := range keys {
			if k.Secret != "" && strings.Contains(v.Group+v.GroupName, k.Secret) {
				valid = false
			}
		}
		if valid && v.Multiplier != nil {
			n, err := strconv.ParseFloat(v.Multiplier.String(), 64)
			valid = err == nil && len(v.Multiplier.String()) <= 128 && n >= 0 && !math.IsNaN(n) && !math.IsInf(n, 0)
			if valid {
				rate.Multiplier = &n
				rate.Group = v.Group
				rate.GroupName = v.GroupName
				rate.Basis = v.Basis
			}
		}
		if !valid {
			result.Failure = "invalid_response"
			result.EffectiveRate = nil
			rate = catalog.KeyRateCache{Failure: "invalid_response"}
		}
	}
	return s.Store.CacheKeyRate(p.ID, p.Revision, keyID, rate)
}
