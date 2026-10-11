package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// PutKeyModelProtocol sets an override for an actual upstream model ID under one
// saved key. An empty protocol (or "inherit") removes it, so future key changes
// are inherited rather than copied into the model.
func (s *Store) PutKeyModelProtocol(id string, revision int64, keyID, model, protocol string) error {
	if protocol == "inherit" {
		protocol = ""
	}
	if !ValidID(keyID) || model == "" || len(model) > 256 || !utf8.ValidString(model) || strings.TrimSpace(model) != model || strings.ContainsFunc(model, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) || (protocol != "" && !ValidProtocol(protocol)) {
		return ErrInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var doc string
	var current int64
	if err = tx.QueryRow(`SELECT document,revision FROM providers WHERE id=?`, id).Scan(&doc, &current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if current != revision {
		return ErrConflict
	}
	var exists int
	if err = tx.QueryRow(`SELECT count(*) FROM provider_keys WHERE provider_id=? AND id=?`, id, keyID).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return ErrNotFound
	}
	if protocol == "" {
		_, err = tx.Exec(`DELETE FROM key_model_protocols WHERE provider_id=? AND key_id=? AND model=?`, id, keyID, model)
	} else {
		_, err = tx.Exec(`INSERT INTO key_model_protocols VALUES(?,?,?,?) ON CONFLICT(provider_id,key_id,model) DO UPDATE SET protocol=excluded.protocol`, id, keyID, model, protocol)
	}
	if err != nil {
		return err
	}
	var p Provider
	if json.Unmarshal([]byte(doc), &p) != nil {
		return ErrInvalid
	}
	p.Revision = current + 1
	raw, _ := json.Marshal(p)
	if _, err = tx.Exec(`UPDATE providers SET document=?,revision=? WHERE id=?`, string(raw), p.Revision, id); err != nil {
		return err
	}
	return tx.Commit()
}
