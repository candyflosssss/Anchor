package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
)

// EffectiveKeyMultiplier deliberately ignores the legacy candidate/model factor.
// That field is reserved for future use; unknown group rates stay unknown.
func (p Provider) EffectiveKeyMultiplier(k ProviderKey) *float64 {
	if k.Rate == nil || k.Rate.Multiplier == nil {
		return nil
	}
	cost := p.CostMultiplier
	if cost == 0 {
		cost = 1
	}
	v := cost * *k.Rate.Multiplier
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

// CacheKeyRate does not increment configuration revisions. Its transaction rejects
// late responses after any provider/Key edit, including credential replacement.
func (s *Store) CacheKeyRate(id string, revision int64, key string, rate catalog.KeyRateCache) error {
	if rate.Multiplier != nil && (math.IsNaN(*rate.Multiplier) || math.IsInf(*rate.Multiplier, 0) || *rate.Multiplier < 0) {
		return ErrInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var rev int64
	if err = tx.QueryRow(`SELECT revision FROM providers WHERE id=?`, id).Scan(&rev); err != nil {
		return err
	}
	if rev != revision {
		return ErrConflict
	}
	var oldDoc string
	err = tx.QueryRow(`SELECT document FROM key_rate_cache WHERE provider_id=? AND key_id=?`, id, key).Scan(&oldDoc)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if rate.Failure != "" || rate.Multiplier == nil {
		failure := rate.Failure
		if failure == "" {
			failure = "unknown"
		}
		rate = catalog.KeyRateCache{}
		if oldDoc != "" && json.Unmarshal([]byte(oldDoc), &rate) != nil {
			return ErrInvalid
		}
		rate.Failure = failure
	} else {
		rate.ObservedAt = now
	}
	rate.AttemptedAt = now
	b, err := json.Marshal(rate)
	if err != nil {
		return err
	}
	// Anonymous sources have no upstream group or cache row.
	_, err = tx.Exec(`INSERT INTO key_rate_cache(provider_id,key_id,document) SELECT ?,?,? WHERE EXISTS(SELECT 1 FROM provider_keys WHERE provider_id=? AND id=?) ON CONFLICT(provider_id,key_id) DO UPDATE SET document=excluded.document`, id, key, string(b), id, key)
	if err != nil {
		return err
	}
	return tx.Commit()
}
