package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
)

// Binding is an approved, local-user configuration target, not an Agent process.
// Credentials are held separately and never appear in the management document.
type Binding struct {
	ID             string `json:"id"`
	AdapterID      string `json:"adapter_id"`
	AdapterVersion string `json:"adapter_version"`
	ConfigRoot     string `json:"config_root"`
	Model          string `json:"model"` // Default, not the permission boundary.
	ModelScope     string `json:"model_scope,omitempty"`
	CodexVersion   string `json:"codex_version,omitempty"`
	Executable     string `json:"anchor_executable"`
	Enabled        bool   `json:"enabled"`
	Revision       int64  `json:"revision"`
}

func (b Binding) Validate() error {
	if b.ModelScope != "" && b.ModelScope != "single" && b.ModelScope != "all-compatible" {
		return errors.New("invalid model scope")
	}
	if (b.ModelScope == "all-compatible" || b.CodexVersion != "") && !ValidCodexVersion(b.CodexVersion) {
		return errors.New("all-compatible bindings require a Codex catalog version")
	}
	if !ValidID(b.ID) || len(b.ID) > 64 || b.AdapterID != "codex" || b.AdapterVersion != "1" || !ValidID(b.Model) || !filepath.IsAbs(b.ConfigRoot) || !filepath.IsAbs(b.Executable) {
		return errors.New("invalid binding: codex v1, absolute approved root/executable and valid IDs required")
	}
	return nil
}
func (s *Store) Bindings() ([]Binding, error) {
	rows, e := s.db.Query(`SELECT document FROM bindings ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Binding{}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			return nil, e
		}
		var b Binding
		if e = json.Unmarshal([]byte(v), &b); e != nil {
			return nil, e
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (s *Store) Binding(id string) (Binding, error) {
	var v string
	e := s.db.QueryRow(`SELECT document FROM bindings WHERE id=?`, id).Scan(&v)
	if errors.Is(e, sql.ErrNoRows) {
		return Binding{}, ErrNotFound
	}
	var b Binding
	if e == nil {
		e = json.Unmarshal([]byte(v), &b)
	}
	return b, e
}
func (s *Store) AddBinding(b Binding) (Binding, error) {
	if e := b.Validate(); e != nil {
		return Binding{}, e
	}
	token, e := Token()
	if e != nil {
		return Binding{}, e
	}
	rootKey := filepath.Clean(b.ConfigRoot)
	if runtime.GOOS == "windows" {
		rootKey = strings.ToLower(rootKey)
	}
	b.Revision = 1
	v, _ := json.Marshal(b)
	tx, e := s.db.Begin()
	if e != nil {
		return Binding{}, e
	}
	defer tx.Rollback()
	// Conservative v1 ownership: one binding for the entire canonical config root.
	var n int
	e = tx.QueryRow(`SELECT count(*) FROM bindings WHERE id=? OR root=?`, b.ID, rootKey).Scan(&n)
	if e != nil {
		return Binding{}, e
	}
	if n != 0 {
		return Binding{}, ErrConflict
	}
	_, e = tx.Exec(`INSERT INTO bindings(id,root,document,token,hash,enabled,revision) VALUES(?,?,?,?,?,?,1)`, b.ID, rootKey, string(v), token, digest(token), b.Enabled)
	if e != nil {
		return Binding{}, e
	}
	e = tx.Commit()
	return b, e
}
func (s *Store) EnableBinding(id string, expected int64, enabled bool) (Binding, error) {
	tx, e := s.db.Begin()
	if e != nil {
		return Binding{}, e
	}
	defer tx.Rollback()
	var v string
	e = tx.QueryRow(`SELECT document FROM bindings WHERE id=?`, id).Scan(&v)
	if errors.Is(e, sql.ErrNoRows) {
		return Binding{}, ErrNotFound
	}
	if e != nil {
		return Binding{}, e
	}
	var b Binding
	if e = json.Unmarshal([]byte(v), &b); e != nil {
		return Binding{}, e
	}
	if expected != b.Revision {
		return Binding{}, ErrConflict
	}
	b.Enabled = enabled
	b.Revision++
	raw, _ := json.Marshal(b)
	_, e = tx.Exec(`UPDATE bindings SET document=?,enabled=?,revision=? WHERE id=?`, string(raw), enabled, b.Revision, id)
	if e != nil {
		return Binding{}, e
	}
	return b, tx.Commit()
}
func (s *Store) BindingToken(id string) (string, error) {
	var token string
	e := s.db.QueryRow(`SELECT token FROM bindings WHERE id=? AND enabled=1`, id).Scan(&token)
	if errors.Is(e, sql.ErrNoRows) {
		e = ErrNotFound
	}
	return token, e
}
func (s *Store) BindingForToken(token string) (Binding, error) {
	var v string
	e := s.db.QueryRow(`SELECT document FROM bindings WHERE hash=? AND enabled=1`, digest(token)).Scan(&v)
	if errors.Is(e, sql.ErrNoRows) {
		return Binding{}, ErrNotFound
	}
	var b Binding
	if e == nil {
		e = json.Unmarshal([]byte(v), &b)
	}
	return b, e
}

// SetBindingModels changes policy without rotating the credential. Management
// restores the old configuration first, and leaves the new revision pending.
func (s *Store) SetBindingModels(id string, expected int64, model, scope, version string) (Binding, error) {
	b, err := s.Binding(id)
	if err != nil {
		return b, err
	}
	if b.Revision != expected {
		return b, ErrConflict
	}
	b.Model = model
	b.ModelScope = scope
	b.CodexVersion = version
	b.Revision++
	if err = b.Validate(); err != nil {
		return b, err
	}
	raw, _ := json.Marshal(b)
	res, err := s.db.Exec(`UPDATE bindings SET document=?,revision=? WHERE id=? AND revision=?`, string(raw), b.Revision, id, expected)
	if err != nil {
		return b, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return b, err
	}
	if n != 1 {
		return b, ErrConflict
	}
	return b, nil
}

// BindingModels is shared by catalog generation and live gateway authorization.
// Legacy/single bindings retain their exact-ID policy. all-compatible is a
// dynamic policy restricted to declared tools/streaming, matching metadata and
// at least one enabled provider. No network probes or secrets are returned.
func (s *Store) BindingModels(b Binding) ([]Model, error) {
	if b.ModelScope != "all-compatible" {
		m, err := s.Model(b.Model)
		if err != nil {
			return nil, err
		}
		return []Model{m}, nil
	}
	models, err := s.Models()
	if err != nil {
		return nil, err
	}
	providers, err := s.Providers()
	if err != nil {
		return nil, err
	}
	enabled := map[string]bool{}
	for _, p := range providers {
		enabled[p.ID] = p.Enabled
	}
	out := []Model{}
	for _, m := range models {
		if !m.Tools || !m.Streaming || m.Codex == nil || m.Codex.Version != b.CodexVersion || m.Codex.Validate() != nil {
			continue
		}
		for _, c := range m.Candidates {
			if enabled[c.ProviderID] {
				out = append(out, m)
				break
			}
		}
	}
	return out, nil
}
func (s *Store) BindingAllows(b Binding, id string) (bool, error) {
	if b.ModelScope != "all-compatible" {
		return id == b.Model, nil
	}
	models, err := s.BindingModels(b)
	if err != nil {
		return false, err
	}
	for _, m := range models {
		if m.ID == id {
			return true, nil
		}
	}
	return false, nil
}
