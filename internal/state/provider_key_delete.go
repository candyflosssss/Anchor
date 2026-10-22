package state

import (
	"database/sql"
	"encoding/json"
)

// deleteProviderKey removes only this provider's Key and its mapping sources in
// the caller's transaction. Never persist an emptied explicit KeyIDs list: an
// empty list means all Keys, which would broaden the mapping's routing scope.
func deleteProviderKey(tx *sql.Tx, providerID, keyID string) error {
	result, err := tx.Exec(`DELETE FROM provider_keys WHERE provider_id=? AND id=?`, providerID, keyID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	var remainingKeys int
	if err = tx.QueryRow(`SELECT count(*) FROM provider_keys WHERE provider_id=?`, providerID).Scan(&remainingKeys); err != nil {
		return err
	}
	return pruneProviderSources(tx, providerID, keyID, remainingKeys)
}

// An empty keyID removes the entire provider; otherwise only that Key is pruned.
// Disabled sources are deliberately retained: only deletion changes mappings.
func pruneProviderSources(tx *sql.Tx, providerID, keyID string, remainingKeys int) error {
	rows, err := tx.Query(`SELECT document,revision FROM models`)
	if err != nil {
		return err
	}
	var changed []Model
	for rows.Next() {
		var document string
		var m Model
		var revision int64
		if err = rows.Scan(&document, &revision); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal([]byte(document), &m); err != nil {
			rows.Close()
			return err
		}
		m.Revision = revision
		candidates := make([]Candidate, 0, len(m.Candidates))
		affected := false
		for _, c := range m.Candidates {
			if c.ProviderID != providerID {
				candidates = append(candidates, c)
				continue
			}
			if keyID == "" {
				affected = true
				continue
			}
			if len(c.KeyIDs) == 0 {
				// An all-Keys candidate retains other saved Keys, but must not
				// silently become anonymous when its last saved Key is removed.
				if remainingKeys > 0 {
					candidates = append(candidates, c)
				} else {
					affected = true
				}
				continue
			}
			keys := make([]string, 0, len(c.KeyIDs))
			for _, key := range c.KeyIDs {
				if key != keyID {
					keys = append(keys, key)
				}
			}
			if len(keys) != len(c.KeyIDs) {
				affected = true
				c.KeyIDs = keys
			}
			if len(c.KeyIDs) > 0 {
				candidates = append(candidates, c)
			}
		}
		refs := make([]SourceRoute, 0, len(m.SourceOrder))
		for _, ref := range m.SourceOrder {
			if ref.ProviderID == providerID && (keyID == "" || ref.KeyID == keyID) {
				affected = true
				continue
			}
			refs = append(refs, ref)
		}
		m.SourceOrder = refs
		if affected {
			m.Candidates = candidates
			changed = append(changed, m)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, m := range changed {
		if len(m.Candidates) == 0 {
			if err = deleteModelTx(tx, m.ID); err != nil {
				return err
			}
			continue
		}
		m.Revision++
		document, err := json.Marshal(m)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE models SET document=?,revision=? WHERE id=?`, string(document), m.Revision, m.ID); err != nil {
			return err
		}
		hasProvider := false
		for _, c := range m.Candidates {
			hasProvider = hasProvider || c.ProviderID == providerID
		}
		if !hasProvider {
			if _, err = tx.Exec(`DELETE FROM provider_prices WHERE provider_id=? AND model=?`, providerID, m.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
