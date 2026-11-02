package state

import (
	"database/sql"
	"encoding/json"
	"errors"
)

const DefaultCodexRetries = 5
const MaxCodexRetries = 10
const DefaultCodexCompactPercent = 90

type CodexSettings struct {
	Retries        int   `json:"retries"`
	AutoCompact    bool  `json:"auto_compact"`
	CompactPercent int   `json:"compact_percent"`
	Revision       int64 `json:"revision"`
}
type SaveCodexSettings struct {
	Retries          *int  `json:"retries"`
	AutoCompact      *bool `json:"auto_compact"`
	CompactPercent   *int  `json:"compact_percent"`
	ExpectedRevision int64 `json:"expected_revision"`
}

func DefaultCodexSettings() CodexSettings {
	return CodexSettings{Retries: DefaultCodexRetries, AutoCompact: true, CompactPercent: DefaultCodexCompactPercent, Revision: 1}
}

func (in SaveCodexSettings) Validate() error {
	if (in.Retries == nil && in.AutoCompact == nil && in.CompactPercent == nil) ||
		(in.Retries != nil && (*in.Retries < 0 || *in.Retries > MaxCodexRetries)) ||
		(in.CompactPercent != nil && (*in.CompactPercent < 1 || *in.CompactPercent > 99)) ||
		in.ExpectedRevision < 1 || in.ExpectedRevision >= 9007199254740991 {
		return ErrInvalid
	}
	return nil
}
func readCodexSettings(q interface{ QueryRow(string, ...any) *sql.Row }) (CodexSettings, error) {
	out := DefaultCodexSettings()
	var raw string
	err := q.QueryRow(`SELECT value FROM meta WHERE key='codex_settings'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if json.Unmarshal([]byte(raw), &out) != nil || out.Retries < 0 || out.Retries > MaxCodexRetries || out.CompactPercent < 1 || out.CompactPercent > 99 || out.Revision < 1 {
		return CodexSettings{}, ErrInvalid
	}
	return out, nil
}
func (s *Store) CodexSettings() (CodexSettings, error) { return readCodexSettings(s.db) }
func (s *Store) SaveCodexSettings(in SaveCodexSettings) (CodexSettings, error) {
	if err := in.Validate(); err != nil {
		return CodexSettings{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return CodexSettings{}, err
	}
	defer tx.Rollback()
	out, err := readCodexSettings(tx)
	if err != nil {
		return out, err
	}
	if out.Revision != in.ExpectedRevision {
		return out, ErrConflict
	}
	if in.Retries != nil {
		out.Retries = *in.Retries
	}
	if in.AutoCompact != nil {
		out.AutoCompact = *in.AutoCompact
	}
	if in.CompactPercent != nil {
		out.CompactPercent = *in.CompactPercent
	}
	out.Revision++
	raw, err := json.Marshal(out)
	if err != nil {
		return CodexSettings{}, err
	}
	_, err = tx.Exec(`INSERT INTO meta(key,value) VALUES('codex_settings',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw))
	if err != nil {
		return CodexSettings{}, err
	}
	return out, tx.Commit()
}
