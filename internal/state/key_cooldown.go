package state

import (
	"database/sql"
	"encoding/json"
	"errors"
)

type KeyCooldownSettings struct {
	Enabled          bool  `json:"enabled"`
	FailureThreshold int   `json:"failure_threshold"`
	CooldownSeconds  int   `json:"cooldown_seconds"`
	RateLimitSeconds int   `json:"rate_limit_seconds"`
	Revision         int64 `json:"revision"`
}
type SaveKeyCooldownSettings struct {
	Enabled          *bool `json:"enabled"`
	FailureThreshold *int  `json:"failure_threshold"`
	CooldownSeconds  *int  `json:"cooldown_seconds"`
	RateLimitSeconds *int  `json:"rate_limit_seconds"`
	ExpectedRevision int64 `json:"expected_revision"`
}

func DefaultKeyCooldownSettings() KeyCooldownSettings {
	return KeyCooldownSettings{Enabled: true, FailureThreshold: 3, CooldownSeconds: 30, RateLimitSeconds: 30, Revision: 1}
}
func (in SaveKeyCooldownSettings) Validate() error {
	if in.Enabled == nil || in.FailureThreshold == nil || *in.FailureThreshold < 1 || *in.FailureThreshold > 100 || in.CooldownSeconds == nil || *in.CooldownSeconds < 1 || *in.CooldownSeconds > 3600 || in.RateLimitSeconds == nil || *in.RateLimitSeconds < 1 || *in.RateLimitSeconds > 3600 || in.ExpectedRevision < 1 || in.ExpectedRevision >= 9007199254740991 {
		return ErrInvalid
	}
	return nil
}
func readKeyCooldownSettings(q interface{ QueryRow(string, ...any) *sql.Row }) (KeyCooldownSettings, error) {
	out := DefaultKeyCooldownSettings()
	var raw string
	err := q.QueryRow(`SELECT value FROM meta WHERE key='key_cooldown_settings'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if json.Unmarshal([]byte(raw), &out) != nil {
		return KeyCooldownSettings{}, ErrInvalid
	}
	in := SaveKeyCooldownSettings{&out.Enabled, &out.FailureThreshold, &out.CooldownSeconds, &out.RateLimitSeconds, min(out.Revision, 9007199254740990)}
	if out.Revision < 1 || out.Revision > 9007199254740991 || in.Validate() != nil {
		return KeyCooldownSettings{}, ErrInvalid
	}
	return out, nil
}
func (s *Store) KeyCooldownSettings() (KeyCooldownSettings, error) {
	return readKeyCooldownSettings(s.db)
}
func (s *Store) SaveKeyCooldownSettings(in SaveKeyCooldownSettings) (KeyCooldownSettings, error) {
	if err := in.Validate(); err != nil {
		return KeyCooldownSettings{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return KeyCooldownSettings{}, err
	}
	defer tx.Rollback()
	old, err := readKeyCooldownSettings(tx)
	if err != nil {
		return old, err
	}
	if old.Revision != in.ExpectedRevision {
		return old, ErrConflict
	}
	out := KeyCooldownSettings{*in.Enabled, *in.FailureThreshold, *in.CooldownSeconds, *in.RateLimitSeconds, old.Revision + 1}
	raw, err := json.Marshal(out)
	if err != nil {
		return KeyCooldownSettings{}, err
	}
	_, err = tx.Exec(`INSERT INTO meta(key,value) VALUES('key_cooldown_settings',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw))
	if err != nil {
		return KeyCooldownSettings{}, err
	}
	return out, tx.Commit()
}
