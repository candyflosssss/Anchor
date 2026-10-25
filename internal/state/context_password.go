package state

import (
	"database/sql"
	"encoding/hex"
	"errors"
)

// ContextPasswordRef is public metadata only. The password lives in the OS vault.
func (s *Store) ContextPasswordRef(name string) (string, error) {
	if !ValidID(name) {
		return "", ErrInvalid
	}
	var ref string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key=?`, "ssh_password:"+name).Scan(&ref)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	b, err := hex.DecodeString(ref)
	if err != nil || len(b) != 32 {
		return "", ErrInvalid
	}
	return ref, nil
}

func (s *Store) SetContextPasswordRef(name, ref string) error {
	if !ValidID(name) {
		return ErrInvalid
	}
	if ref == "" {
		_, err := s.db.Exec(`DELETE FROM meta WHERE key=?`, "ssh_password:"+name)
		return err
	}
	b, err := hex.DecodeString(ref)
	if err != nil || len(b) != 32 {
		return ErrInvalid
	}
	_, err = s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "ssh_password:"+name, ref)
	return err
}
