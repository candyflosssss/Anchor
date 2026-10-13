package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"math"
	"strings"
	"time"
)

var ErrReferenced = errors.New("resource is referenced")
var ErrInvalid = errors.New("invalid operation")

type ProviderKey struct {
	Protocol       string                  `json:"protocol"`
	ModelProtocols []catalog.ModelProtocol `json:"model_protocols"`
	ID             string                  `json:"id"`
	Enabled        bool                    `json:"enabled"`
	Secret         string                  `json:"-"`
}
type Usage struct {
	ProviderID   string `json:"provider_id"`
	Model        string `json:"model"`
	Requests     int64  `json:"requests"`
	Failures     int64  `json:"failures"`
	ElapsedMS    int64  `json:"elapsed_ms"`
	Known        int64  `json:"known"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read"`
	CacheWrite   int64  `json:"cache_write"`
}

func (k ProviderKey) EffectiveProtocol(model string) string {
	for _, override := range k.ModelProtocols {
		if override.Model == model {
			return override.Protocol
		}
	}
	return k.Protocol
}

func (s *Store) ProviderKeys(id string) ([]ProviderKey, error) {
	rows, err := s.db.Query(`SELECT k.id,k.enabled,k.secret,k.protocol,COALESCE(m.model,''),COALESCE(m.protocol,'')
 FROM provider_keys k LEFT JOIN key_model_protocols m ON m.provider_id=k.provider_id AND m.key_id=k.id
 WHERE k.provider_id=? ORDER BY k.id,m.model`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProviderKey{}
	for rows.Next() {
		var k ProviderKey
		var model, protocol string
		if err = rows.Scan(&k.ID, &k.Enabled, &k.Secret, &k.Protocol, &model, &protocol); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].ID != k.ID {
			k.ModelProtocols = []catalog.ModelProtocol{}
			out = append(out, k)
		}
		if model != "" {
			out[len(out)-1].ModelProtocols = append(out[len(out)-1].ModelProtocols, catalog.ModelProtocol{Model: model, Protocol: protocol})
		}
	}
	return out, rows.Err()
}
func (s *Store) KeyStrategy(id string) string {
	var v string
	_ = s.db.QueryRow(`SELECT strategy FROM provider_strategy WHERE provider_id=?`, id).Scan(&v)
	if v == "priority" {
		return v
	}
	return "round-robin"
}

// MutateProvider holds an IMMEDIATE transaction across revision and reference checks.
func (s *Store) MutateProvider(id string, revision int64, action, target, secret, strategy string, enabled, copyKeys bool, keyProtocol ...string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var document, oldSecret string
	var current int64
	err = tx.QueryRow(`SELECT document,secret,revision FROM providers WHERE id=?`, id).Scan(&document, &oldSecret, &current)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if current != revision {
		return ErrConflict
	}
	var p Provider
	if json.Unmarshal([]byte(document), &p) != nil {
		return ErrInvalid
	}
	switch action {
	case "delete", "rename", "copy":
		if action != "delete" {
			if !ValidProviderID(target) || target == id {
				return ErrInvalid
			}
			var n int
			if err = tx.QueryRow(`SELECT count(*) FROM providers WHERE id=?`, target).Scan(&n); err != nil {
				return err
			}
			if n != 0 {
				return ErrConflict
			}
		}
		rows, e := tx.Query(`SELECT document,revision FROM models`)
		if e != nil {
			return e
		}
		var changed []Model
		for rows.Next() {
			var d string
			var rev int64
			if e = rows.Scan(&d, &rev); e != nil {
				rows.Close()
				return e
			}
			var m Model
			if json.Unmarshal([]byte(d), &m) != nil {
				rows.Close()
				return ErrInvalid
			}
			m.Revision = rev
			used := false
			for i := range m.Candidates {
				if m.Candidates[i].ProviderID == id {
					used = true
					m.Candidates[i].ProviderID = target
				}
			}
			if used {
				changed = append(changed, m)
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if action == "delete" {
			if len(changed) > 0 {
				return ErrReferenced
			}
			_, err = tx.Exec(`DELETE FROM providers WHERE id=?`, id)
		}
		if action == "rename" {
			p.ID = target
			p.Revision = current + 1
			b, _ := json.Marshal(p)
			_, err = tx.Exec(`UPDATE providers SET id=?,document=?,revision=? WHERE id=?`, target, string(b), p.Revision, id)
			if err != nil {
				return err
			}
			for _, m := range changed {
				m.Revision++
				b, _ := json.Marshal(m)
				if _, err = tx.Exec(`UPDATE models SET document=?,revision=? WHERE id=?`, string(b), m.Revision, m.ID); err != nil {
					return err
				}
			}
		}
		if action == "copy" {
			p.ID = target
			p.Revision = 1
			p.Enabled = false
			p.HasSecret = copyKeys && p.HasSecret
			if !copyKeys {
				oldSecret = ""
			}
			b, _ := json.Marshal(p)
			_, err = tx.Exec(`INSERT INTO providers VALUES(?,?,?,1)`, target, string(b), oldSecret)
			if err != nil {
				return err
			}
			// Copy platform identity, but dashboard user authorization is a separate
			// credential and is never included in copyKeys.
			_, err = tx.Exec(`INSERT INTO provider_platforms SELECT ?,document,'' FROM provider_platforms WHERE provider_id=?`, target, id)
			if err != nil {
				return err
			}
			if copyKeys {
				_, err = tx.Exec(`INSERT INTO provider_keys SELECT ?,id,secret,enabled,protocol FROM provider_keys WHERE provider_id=?`, target, id)
			}
			if err == nil && copyKeys {
				_, err = tx.Exec(`INSERT INTO key_model_protocols SELECT ?,key_id,model,protocol FROM key_model_protocols WHERE provider_id=?`, target, id)
			}
			if err == nil {
				_, err = tx.Exec(`INSERT INTO provider_strategy SELECT ?,strategy FROM provider_strategy WHERE provider_id=?`, target, id)
			}
		}
	case "key-put", "key-delete", "strategy":
		if action == "strategy" {
			if strategy != "round-robin" && strategy != "priority" {
				return ErrInvalid
			}
			_, err = tx.Exec(`INSERT INTO provider_strategy VALUES(?,?) ON CONFLICT(provider_id) DO UPDATE SET strategy=excluded.strategy`, id, strategy)
		} else {
			if !ValidID(target) {
				return ErrInvalid
			}
			if action == "key-delete" {
				if err = checkKeyUnreferenced(tx, id, target); err != nil {
					return err
				}
				var res sql.Result
				res, err = tx.Exec(`DELETE FROM provider_keys WHERE provider_id=? AND id=?`, id, target)
				if err == nil {
					n, _ := res.RowsAffected()
					if n != 1 {
						return ErrNotFound
					}
				}
			} else {
				if strings.ContainsAny(secret, "\r\n") || len(secret) > 8192 {
					return ErrInvalid
				}
				if secret == "" {
					var previous string
					err = tx.QueryRow(`SELECT secret FROM provider_keys WHERE provider_id=? AND id=?`, id, target).Scan(&previous)
					if err != nil {
						return ErrInvalid
					}
					secret = previous
				}
				var n int
				if err = tx.QueryRow(`SELECT count(*) FROM provider_keys WHERE provider_id=?`, id).Scan(&n); err != nil {
					return err
				}
				if n >= 64 {
					var exists int
					_ = tx.QueryRow(`SELECT count(*) FROM provider_keys WHERE provider_id=? AND id=?`, id, target).Scan(&exists)
					if exists == 0 {
						return ErrInvalid
					}
				}
				protocol := ""
				if len(keyProtocol) > 0 {
					protocol = keyProtocol[0]
				}
				if protocol == "" {
					e := tx.QueryRow(`SELECT protocol FROM provider_keys WHERE provider_id=? AND id=?`, id, target).Scan(&protocol)
					if errors.Is(e, sql.ErrNoRows) {
						protocol = "responses"
					} else if e != nil {
						return e
					}
				}
				if !ValidProtocol(protocol) {
					return ErrInvalid
				}
				_, err = tx.Exec(`INSERT INTO provider_keys(provider_id,id,secret,enabled,protocol) VALUES(?,?,?,?,?) ON CONFLICT(provider_id,id) DO UPDATE SET secret=excluded.secret,enabled=excluded.enabled,protocol=excluded.protocol`, id, target, secret, enabled, protocol)
			}
		}
		if err != nil {
			return err
		}
		var n int
		_ = tx.QueryRow(`SELECT count(*) FROM provider_keys WHERE provider_id=?`, id).Scan(&n)
		p.HasSecret = n > 0
		p.Revision = current + 1
		primary := ""
		e := tx.QueryRow(`SELECT secret FROM provider_keys WHERE provider_id=? AND id='default'`, id).Scan(&primary)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		b, _ := json.Marshal(p)
		_, err = tx.Exec(`UPDATE providers SET document=?,secret=?,revision=? WHERE id=?`, string(b), primary, p.Revision, id)
	default:
		return ErrInvalid
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) DeleteModel(id string, revision int64) error {
	return s.DeleteProviderModel(id, revision, "", 0)
}
func (s *Store) DeleteProviderModel(id string, revision int64, providerID string, providerRevision int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if providerID != "" {
		var current int64
		if err = tx.QueryRow(`SELECT revision FROM providers WHERE id=?`, providerID).Scan(&current); err != nil {
			return ErrNotFound
		}
		if current != providerRevision {
			return ErrConflict
		}
	}
	var rev int64
	var document string
	err = tx.QueryRow(`SELECT revision,document FROM models WHERE id=?`, id).Scan(&rev, &document)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if rev != revision {
		return ErrConflict
	}
	if providerID != "" {
		var m Model
		if err = json.Unmarshal([]byte(document), &m); err != nil {
			return err
		}
		remaining := make([]Candidate, 0, len(m.Candidates))
		for _, c := range m.Candidates {
			if c.ProviderID != providerID {
				remaining = append(remaining, c)
			}
		}
		if len(remaining) == len(m.Candidates) {
			return ErrInvalid
		}
		if len(remaining) > 0 {
			m.Candidates, m.Revision = remaining, rev+1
			b, e := json.Marshal(m)
			if e != nil {
				return e
			}
			if _, err = tx.Exec(`UPDATE models SET document=?,revision=? WHERE id=? AND revision=?`, string(b), m.Revision, id, rev); err != nil {
				return err
			}
			if _, err = tx.Exec(`DELETE FROM provider_prices WHERE provider_id=? AND model=?`, providerID, id); err != nil {
				return err
			}
			return tx.Commit()
		}
	}
	rows, err := tx.Query(`SELECT document FROM bindings`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var d string
		if err = rows.Scan(&d); err != nil {
			rows.Close()
			return err
		}
		var b Binding
		if json.Unmarshal([]byte(d), &b) != nil {
			rows.Close()
			return ErrInvalid
		}
		if b.Model == id {
			rows.Close()
			return ErrReferenced
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM provider_prices WHERE model=?`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM models WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) RecordUsage(u Usage) error {
	_, err := s.db.Exec(`INSERT INTO usage_totals VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(provider_id,model) DO UPDATE SET requests=requests+excluded.requests,failures=failures+excluded.failures,elapsed_ms=elapsed_ms+excluded.elapsed_ms,known=known+excluded.known,input_tokens=input_tokens+excluded.input_tokens,output_tokens=output_tokens+excluded.output_tokens,cache_read=cache_read+excluded.cache_read,cache_write=cache_write+excluded.cache_write`, u.ProviderID, u.Model, u.Requests, u.Failures, u.ElapsedMS, u.Known, u.InputTokens, u.OutputTokens, u.CacheRead, u.CacheWrite)
	return err
}
func (s *Store) Usage(id string) ([]Usage, error) {
	rows, err := s.db.Query(`SELECT * FROM usage_totals WHERE (?='' OR provider_id=?) ORDER BY provider_id,model`, id, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Usage{}
	for rows.Next() {
		var u Usage
		if err = rows.Scan(&u.ProviderID, &u.Model, &u.Requests, &u.Failures, &u.ElapsedMS, &u.Known, &u.InputTokens, &u.OutputTokens, &u.CacheRead, &u.CacheWrite); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) Prices(id string) ([]catalog.Price, error) {
	rows, err := s.db.Query(`SELECT document FROM provider_prices WHERE provider_id=? ORDER BY model`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []catalog.Price{}
	for rows.Next() {
		var b string
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		var p catalog.Price
		if err = json.Unmarshal([]byte(b), &p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) PutPrice(id string, revision int64, p catalog.Price) error {
	if !ValidID(p.Model) || len(p.Source) > 256 || strings.ContainsAny(p.Source, "\r\n") {
		return ErrInvalid
	}
	values := []float64{p.InputUSD, p.OutputUSD}
	if p.CacheReadUSD != nil {
		values = append(values, *p.CacheReadUSD)
	}
	if p.CacheWriteUSD != nil {
		values = append(values, *p.CacheWriteUSD)
	}
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1e6 {
			return ErrInvalid
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var rev int64
	var doc string
	if err = tx.QueryRow(`SELECT revision,document FROM providers WHERE id=?`, id).Scan(&rev, &doc); err != nil {
		return err
	}
	if rev != revision {
		return ErrConflict
	}
	var m Model
	var modelDoc string
	if err = tx.QueryRow(`SELECT document FROM models WHERE id=?`, p.Model).Scan(&modelDoc); err != nil {
		return ErrNotFound
	}
	if json.Unmarshal([]byte(modelDoc), &m) != nil {
		return ErrInvalid
	}
	found := false
	for _, c := range m.Candidates {
		if c.ProviderID == id {
			found = true
		}
	}
	if !found {
		return ErrInvalid
	}
	p.UpdatedAt = time.Now().Unix()
	if p.Source == "" {
		p.Source = "custom"
	}
	b, _ := json.Marshal(p)
	if _, err = tx.Exec(`INSERT INTO provider_prices VALUES(?,?,?) ON CONFLICT(provider_id,model) DO UPDATE SET document=excluded.document`, id, p.Model, string(b)); err != nil {
		return err
	}
	var provider Provider
	if json.Unmarshal([]byte(doc), &provider) != nil {
		return ErrInvalid
	}
	provider.Revision = rev + 1
	b, _ = json.Marshal(provider)
	if _, err = tx.Exec(`UPDATE providers SET revision=?,document=? WHERE id=?`, rev+1, string(b), id); err != nil {
		return err
	}
	return tx.Commit()
}
