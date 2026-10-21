package state

import (
	"context"
	"encoding/json"
)

// ProviderImport is internal write-only input. Imports create providers; they
// never replace credentials or configuration belonging to an existing ID.
type ProviderImport struct {
	Provider Provider
	Keys     []InitialProviderKey
}

type ImportedModel struct {
	Model            Model
	ExpectedRevision int64
}

// ImportProviders commits the complete reviewed batch, including model CAS,
// in one transaction. A conflict or validation failure rolls back every row.
func (s *Store) ImportProviders(ctx context.Context, providers []ProviderImport, models []ImportedModel) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range providers {
		p := item.Provider
		if err := p.Validate(); err != nil {
			return ErrInvalid
		}
		if len(item.Keys) == 0 || ValidateInitialProviderKeys(item.Keys, 0, nil) != nil {
			return ErrInvalid
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM providers WHERE id=?`, p.ID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return ErrConflict
		}
		p.Normalize()
		p.Secret = ""
		p.Enabled = true
		p.HasSecret = true
		p.Revision = 1
		b, _ := json.Marshal(p)
		if _, err := tx.ExecContext(ctx, `INSERT INTO providers(id,document,secret,revision) VALUES(?,?,'',1)`, p.ID, string(b)); err != nil {
			return err
		}
		for _, key := range item.Keys {
			if _, err := tx.ExecContext(ctx, `INSERT INTO provider_keys(provider_id,id,secret,enabled,protocol) VALUES(?,?,?,1,?)`, p.ID, key.ID, key.Secret, key.Protocol); err != nil {
				return err
			}
		}
	}
	for _, item := range models {
		if _, err := putModelTx(tx, item.Model, item.ExpectedRevision); err != nil {
			return err
		}
	}
	return tx.Commit()
}
