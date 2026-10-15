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
	// Local catalog snapshots belong to this target/version, not provider routing.
	CodexCatalog   map[string]CodexMetadata `json:"codex_catalog,omitempty"`
	ModelTemplates map[string]string        `json:"model_templates,omitempty"`
}

func (b Binding) Validate() error {
	if b.AdapterID != "codex" && (b.ModelScope == "all-compatible" || b.CodexVersion != "" || len(b.CodexCatalog) > 0 || len(b.ModelTemplates) > 0) {
		return ErrInvalid
	}
	if len(b.CodexCatalog) > 128 || len(b.ModelTemplates) > 256 {
		return ErrInvalid
	}
	size := 0
	for slug, meta := range b.CodexCatalog {
		if slug == "" || len(slug) > 256 || meta.Version != b.CodexVersion || meta.Validate() != nil {
			return ErrInvalid
		}
		var entry struct {
			Slug string `json:"slug"`
		}
		if json.Unmarshal(meta.Entry, &entry) != nil || entry.Slug != slug {
			return ErrInvalid
		}
		size += len(meta.Entry)
	}
	if size > 4<<20 {
		return ErrInvalid
	}
	for alias, source := range b.ModelTemplates {
		if !ValidModelID(alias) {
			return ErrInvalid
		}
		if _, ok := b.CodexCatalog[source]; source != "" && !ok {
			return ErrInvalid
		}
	}

	if b.ModelScope != "" && b.ModelScope != "single" && b.ModelScope != "all-compatible" && b.ModelScope != "all-mapped" {
		return errors.New("invalid model scope")
	}
	if (b.ModelScope == "all-compatible" || b.CodexVersion != "") && !ValidCodexVersion(b.CodexVersion) {
		return errors.New("all-compatible bindings require a Codex catalog version")
	}
	if !ValidID(b.ID) || len(b.ID) > 64 || (b.AdapterID != "codex" && b.AdapterID != "claude" && b.AdapterID != "dsh" && b.AdapterID != "dsh-desktop") || b.AdapterVersion != "1" || !ValidModelID(b.Model) || !filepath.IsAbs(b.ConfigRoot) || !filepath.IsAbs(b.Executable) {
		return errors.New("invalid binding: supported adapter v1, absolute approved root/executable and valid IDs required")
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

// BindingRootsConflict retains whole-root ownership except for DSH's two
// independently managed, disjoint profiles. Disabled bindings retain ownership.
func BindingRootsConflict(a, b Binding) bool {
	if canonicalBindingRoot(a.ConfigRoot) != canonicalBindingRoot(b.ConfigRoot) {
		return false
	}
	return !((a.AdapterID == "dsh" && b.AdapterID == "dsh-desktop") ||
		(a.AdapterID == "dsh-desktop" && b.AdapterID == "dsh"))
}

func canonicalBindingRoot(root string) string {
	root = filepath.Clean(root)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	return root
}

func (s *Store) AddBinding(b Binding) (Binding, error) {
	if e := b.Validate(); e != nil {
		return Binding{}, e
	}
	token, e := Token()
	if e != nil {
		return Binding{}, e
	}
	root := canonicalBindingRoot(b.ConfigRoot)
	rootKey := root
	// Keep all existing keys intact, including legacy Web bindings. The prefix
	// cannot collide with an absolute path; no schema/data migration is needed.
	if b.AdapterID == "dsh-desktop" {
		rootKey = "dsh-desktop:" + root
	}
	b.Revision = 1
	v, _ := json.Marshal(b)
	tx, e := s.db.Begin()
	if e != nil {
		return Binding{}, e
	}
	defer tx.Rollback()
	rows, e := tx.Query(`SELECT document FROM bindings WHERE id=? OR root IN (?,?)`, b.ID, root, "dsh-desktop:"+root)
	if e != nil {
		return Binding{}, e
	}
	defer rows.Close()
	for rows.Next() {
		var document string
		var old Binding
		if e = rows.Scan(&document); e != nil {
			return Binding{}, e
		}
		if e = json.Unmarshal([]byte(document), &old); e != nil {
			return Binding{}, e
		}
		if old.ID == b.ID || BindingRootsConflict(old, b) {
			return Binding{}, ErrConflict
		}
	}
	if e = rows.Err(); e != nil {
		return Binding{}, e
	}
	if e = rows.Close(); e != nil {
		return Binding{}, e
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
	if enabled {
		// Model deletion and activation must agree within the same transaction:
		// disabled bindings may retain a default whose mapping has been removed.
		var exists int
		e = tx.QueryRow(`SELECT 1 FROM models WHERE id=?`, b.Model).Scan(&exists)
		if errors.Is(e, sql.ErrNoRows) {
			return Binding{}, ErrNotFound
		}
		if e != nil {
			return Binding{}, e
		}
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
	if b.ModelScope != "all-compatible" && b.ModelScope != "all-mapped" {
		m, err := s.Model(b.Model)
		if err != nil {
			return nil, err
		}
		m.Codex = BindingMetadata(b, m)
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
		m.Codex = BindingMetadata(b, m)
		if b.ModelScope == "all-compatible" && (!m.Tools || !m.Streaming || m.Codex == nil || m.Codex.Version != b.CodexVersion || m.Codex.Validate() != nil) {
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
	if b.ModelScope != "all-compatible" && b.ModelScope != "all-mapped" {
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

// BindingMetadata resolves explicit choices first, then exact catalog identities.
// It never invents capabilities or changes the provider's shared model document.
func BindingMetadata(b Binding, m Model) *CodexMetadata {
	if source, ok := b.ModelTemplates[m.ID]; ok {
		meta, found := b.CodexCatalog[source]
		if !found {
			return nil
		}
		return &meta
	}
	if m.Codex != nil && (m.Codex.Version == b.CodexVersion || (b.CodexVersion == "" && b.ModelScope != "all-compatible")) {
		return m.Codex
	}
	if meta, ok := b.CodexCatalog[m.ID]; ok {
		return &meta
	}
	source := ""
	for _, c := range m.Candidates {
		if _, ok := b.CodexCatalog[c.Model]; ok {
			if source != "" && source != c.Model {
				return nil
			}
			source = c.Model
		}
	}
	if source == "" {
		return nil
	}
	meta := b.CodexCatalog[source]
	return &meta
}

// UpdateBindingCatalog preserves target ownership and credentials while updating
// the approved target-local model policy and catalog snapshot.
func (s *Store) UpdateBindingCatalog(b Binding, expected int64) (Binding, error) {
	old, err := s.Binding(b.ID)
	if err != nil {
		return b, err
	}
	if old.Revision != expected {
		return b, ErrConflict
	}
	if old.ConfigRoot != b.ConfigRoot || old.Executable != b.Executable || old.AdapterID != b.AdapterID || old.AdapterVersion != b.AdapterVersion {
		return b, ErrInvalid
	}
	b.Revision = expected + 1
	if err = b.Validate(); err != nil {
		return b, err
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return b, err
	}
	res, err := s.db.Exec(`UPDATE bindings SET document=?,enabled=?,revision=? WHERE id=? AND revision=?`, string(raw), b.Enabled, b.Revision, b.ID, expected)
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
