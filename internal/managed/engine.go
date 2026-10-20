package managed

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

const maxConfig = 1 << 20

// Plan contains no private values. The internal edits and original bytes stay
// inside the engine, with durable recovery material under protected storage.
type Plan struct {
	ExpectedRevision  int64      `json:"expected_revision"`
	Model             string     `json:"model"`
	ModelScope        string     `json:"model_scope,omitempty"`
	AvailableModels   []string   `json:"available_models,omitempty"`
	CatalogPath       string     `json:"catalog_path,omitempty"`
	CatalogVersion    string     `json:"catalog_version,omitempty"`
	GatewayURL        string     `json:"gateway_base_url"`
	CredentialMethod  string     `json:"credential_method"`
	BindingID         string     `json:"binding_id"`
	AdapterVersion    string     `json:"adapter_version"`
	Path              string     `json:"path"`
	Fingerprint       string     `json:"fingerprint"`
	Fields            [][]string `json:"fields"`
	Reload            string     `json:"reload_policy"`
	Verification      string     `json:"verification"`
	Edits             []Edit     `json:"-"`
	original          []byte
	existed           bool
	configFingerprint string
	catalog           *catalogArtifact
}
type Result struct {
	BindingID   string     `json:"binding_id"`
	OperationID string     `json:"operation_id,omitempty"`
	Status      string     `json:"status"`
	Conflicts   [][]string `json:"conflicts,omitempty"`
}
type catalogArtifact struct {
	Path     string `json:"path"`
	Written  []byte `json:"written"`
	Original []byte `json:"original,omitempty"`
	Existed  bool   `json:"existed,omitempty"`
}
type journal struct {
	ReplacementFingerprint string           `json:"replacement_fingerprint,omitempty"`
	LastWritten            []byte           `json:"last_written,omitempty"`
	Previous               *previousWrite   `json:"previous,omitempty"`
	Schema                 int              `json:"schema"`
	BindingID              string           `json:"binding_id"`
	AdapterVersion         string           `json:"adapter_version"`
	OperationID            string           `json:"operation_id"`
	Path                   string           `json:"path"`
	Original               []byte           `json:"original"`
	Existed                bool             `json:"existed"`
	Edits                  []Edit           `json:"edits"`
	Status                 string           `json:"status"`
	Catalog                *catalogArtifact `json:"catalog,omitempty"`
}
type Engine struct {
	Dir    string
	Editor Editor
}

func NewEngine(store *state.Store) *Engine {
	return &Engine{Dir: filepath.Join(store.Dir, "recovery"), Editor: TOMLEditor{}}
}
func fingerprint(b []byte, exists bool) string {
	h := sha256.New()
	if exists {
		h.Write([]byte{1})
	}
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
func readConfig(root, path string) ([]byte, bool, error) {
	if e := platform.CheckManagedPath(root, path); e != nil {
		return nil, false, e
	}
	f, e := os.Open(path)
	if errors.Is(e, os.ErrNotExist) {
		return nil, false, nil
	}
	if e != nil {
		return nil, false, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, maxConfig+1))
	if e != nil || len(b) > maxConfig {
		return nil, false, ErrUnsafe
	}
	return b, true, nil
}
func (e *Engine) prepare() error               { return platform.SecureDir(e.Dir) }
func (e *Engine) journalPath(id string) string { return filepath.Join(e.Dir, id+".json") }
func (e *Engine) load(id string) (journal, error) {
	var j journal
	if !state.ValidID(id) {
		return j, ErrUnsafe
	}
	p := e.journalPath(id)
	if _, err := os.Lstat(p); err != nil {
		return j, err
	}
	if err := platform.SecureFile(p); err != nil {
		return j, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return j, err
	}
	if len(b) > 8*maxConfig {
		return j, ErrUnsafe
	}
	if err = json.Unmarshal(b, &j); err != nil {
		return j, ErrUnsafe
	}
	if j.Schema != 1 || j.BindingID != id || j.AdapterVersion != "1" || !supportedConfig(j.Path) || !filepath.IsAbs(j.Path) {
		return j, ErrUnsafe
	}
	if len(j.LastWritten) > maxConfig {
		return j, ErrUnsafe
	}
	if j.ReplacementFingerprint != "" && (len(j.ReplacementFingerprint) != 64 || (j.Status != "prepared" && j.Status != "conflict")) {
		return j, ErrUnsafe
	}
	if j.Catalog != nil && (filepath.Dir(j.Catalog.Path) != filepath.Dir(j.Path) || filepath.Base(j.Catalog.Path) != catalogName(id) || len(j.Catalog.Written) > maxConfig || len(j.Catalog.Original) > maxConfig || !json.Valid(j.Catalog.Written)) {
		return j, ErrUnsafe
	}
	if j.Previous != nil {
		if j.Status != "prepared" && j.Status != "conflict" {
			return j, ErrUnsafe
		}
		if a := j.Previous.Catalog; a != nil && (j.Catalog == nil || a.Path != j.Catalog.Path || len(a.Written) > maxConfig || len(a.Original) > maxConfig || !json.Valid(a.Written)) {
			return j, ErrUnsafe
		}
	}
	return j, nil
}
func (e *Engine) save(j journal) error {
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return atomicWrite(e.Dir, e.journalPath(j.BindingID), b, "", false)
}

// atomicWrite is optimistic, not a lock against external editors. It performs a
// final byte-level comparison and never unconditionally publishes stale data.
func atomicWrite(root, path string, b []byte, expected string, check bool) error {
	if err := platform.CheckManagedPath(root, path); err != nil {
		return err
	}
	f, err := os.CreateTemp(root, ".anchor-write-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	defer f.Close()
	if err = platform.PreserveManagedPermissions(path, tmp); err != nil {
		return err
	}
	if filepath.Base(path) == "settings.json" || filepath.Base(path) == "cordis.patch.yml" {
		if err = platform.SecureFile(tmp); err != nil {
			return err
		}
	}
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if check {
		current, exists, err := readConfig(root, path)
		if err != nil {
			return err
		}
		if fingerprint(current, exists) != expected {
			return withDiagnostic(ErrChanged, "write_configuration", "configuration_changed_before_write", "")
		}
	}
	if err = platform.CheckManagedPath(root, path); err != nil {
		return err
	}
	if err = platform.ReplaceManagedFile(tmp, path); err != nil {
		return err
	}
	return platform.SyncManagedDir(root)
}
func (e *Engine) withLock(fn func() error) error {
	if err := e.prepare(); err != nil {
		return err
	}
	l, err := platform.AcquireLock(filepath.Join(e.Dir, "engine.lock"))
	if err != nil {
		return err
	}
	defer l.Close()
	return fn()
}
func (e *Engine) Apply(p Plan) (Result, error) {
	out := Result{BindingID: p.BindingID, Status: "pending"}
	err := e.withLock(func() error {
		if !state.ValidID(p.BindingID) || p.AdapterVersion != "1" {
			return ErrUnsafe
		}
		if old, err := e.load(p.BindingID); err == nil && old.Status != "restored" {
			out.OperationID = old.OperationID
			out.Status = "recovery_pending"
			reason := "unfinished_journal"
			if old.Status == "applied" {
				reason = "active_journal"
			}
			return withDiagnostic(ErrChanged, "apply_precondition", reason, "")
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		root := filepath.Dir(p.Path)
		b, exists, err := readConfig(root, p.Path)
		if err != nil {
			return err
		}
		expected := p.configFingerprint
		if expected == "" {
			expected = p.Fingerprint
		}
		if fingerprint(b, exists) != expected {
			return withDiagnostic(ErrChanged, "read_configuration", "configuration_changed_since_plan", "")
		}
		if p.catalog != nil {
			if filepath.Dir(p.catalog.Path) != root || filepath.Base(p.catalog.Path) != catalogName(p.BindingID) || len(p.catalog.Written) > maxConfig || !json.Valid(p.catalog.Written) {
				return ErrUnsafe
			}
			_, catalogExists, err := readConfig(root, p.catalog.Path)
			if err != nil {
				return err
			}
			if catalogExists {
				return withDiagnostic(ErrChanged, "apply_precondition", "catalog_exists", "model_catalog_json")
			}
		}
		next, err := e.editor(p.Path).Patch(b, p.Edits)
		if err != nil {
			return err
		}
		op, err := state.Token()
		if err != nil {
			return err
		}
		j := journal{Schema: 1, BindingID: p.BindingID, AdapterVersion: p.AdapterVersion, OperationID: op[:16], Path: p.Path, Original: b, Existed: exists, Edits: p.Edits, Status: "prepared", Catalog: p.catalog, LastWritten: next}
		// Save recovery before the first config write. A crash on either side of
		// rename can be recovered by comparing original and written field values.
		if err = e.save(j); err != nil {
			return err
		}
		out.OperationID = j.OperationID
		out.Status = "recovery_pending"
		if j.Catalog != nil {
			if err = atomicWrite(root, j.Catalog.Path, j.Catalog.Written, fingerprint(nil, false), true); err != nil {
				return err
			}
		}
		if err = atomicWrite(root, p.Path, next, expected, true); err != nil {
			return err
		}
		j.Status = "applied"
		if err = e.save(j); err != nil {
			return err
		}
		out.Status = "applied"
		return nil
	})
	return out, err
}
func (e *Engine) Recover(id string) (Result, error) {
	out := Result{BindingID: id, Status: "unmanaged"}
	err := e.withLock(func() error {
		j, err := e.load(id)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		out.OperationID = j.OperationID
		if j.Status == "restored" {
			out.Status = "restored"
			return nil
		}
		b, exists, err := readConfig(filepath.Dir(j.Path), j.Path)
		if err != nil {
			return err
		}
		// A replacement interrupted before publishing config has not changed
		// these bytes. They may be invalid TOML; do not parse or overwrite them.
		if j.ReplacementFingerprint != "" && fingerprint(b, exists) == j.ReplacementFingerprint {
			if j.Catalog != nil {
				if _, parseErr := document(b); parseErr != nil {
					// Cannot prove that an unreadable untouched config does not
					// reference the catalog. Retain it for explicit rebuilding.
					out.Conflicts = append(out.Conflicts, []string{"catalog", filepath.Base(j.Catalog.Path)})
				} else if err := e.recoverCatalog(j, &out); err != nil {
					return err
				}
			}
			out.Status = "restored"
			if len(out.Conflicts) > 0 {
				out.Status = "conflict"
			} else {
				j.Previous, j.ReplacementFingerprint = nil, ""
			}
			j.Status = out.Status
			return e.save(j)
		}
		paths := [][]string{}
		for _, x := range j.Edits {
			paths = append(paths, x.Path)
		}
		values, err := e.editor(j.Path).Values(b, paths)
		if err != nil {
			return err
		}
		edits := []Edit{}
		for i, x := range j.Edits {
			switch {
			case equal(values[i], x.Before):
			case equal(values[i], x.After) || previousValue(j, x.Path, values[i]):
				edits = append(edits, Edit{Path: x.Path, Before: values[i], After: x.Before})
			default:
				out.Conflicts = append(out.Conflicts, x.Path)
			}
		}
		// A created provider is an artifact: do not delete it while a user
		// profile (or an overridden model_provider) still references it.
		for i := 0; i < len(edits); i++ {
			x := edits[i]
			if len(x.Path) != 2 || x.Path[0] != "model_providers" || x.After.Exists {
				continue
			}
			roots := []Edit{}
			for _, r := range edits {
				if len(r.Path) == 1 {
					roots = append(roots, r)
				}
			}
			preview, err := e.editor(j.Path).Patch(b, roots)
			if err != nil {
				return err
			}
			doc, err := document(preview)
			if err != nil {
				return err
			}
			referenced, err := fileReferences(filepath.Dir(j.Path), "", func(v any) bool { return referencesProvider(v, x.Path[1]) })
			if err != nil {
				return err
			}
			if referencesProvider(doc, x.Path[1]) || referenced {
				out.Conflicts = append(out.Conflicts, x.Path)
				edits = append(edits[:i], edits[i+1:]...)
				i--
			}
		}
		next, err := e.editor(j.Path).Patch(b, edits)
		if err != nil {
			return err
		}
		if len(edits) > 0 {
			if err = atomicWrite(filepath.Dir(j.Path), j.Path, next, fingerprint(b, exists), true); err != nil {
				return err
			}
		}
		if j.Catalog != nil {
			if err = e.recoverCatalog(j, &out); err != nil {
				return err
			}
		}
		out.Status = "restored"
		if len(out.Conflicts) > 0 {
			out.Status = "conflict"
		}
		j.Status = out.Status
		if out.Status == "restored" {
			j.Previous = nil
			j.ReplacementFingerprint = ""
		}
		return e.save(j)
	})
	return out, err
}
func (e *Engine) Status(id string) (Result, error) {
	j, err := e.load(id)
	if errors.Is(err, os.ErrNotExist) {
		return Result{BindingID: id, Status: "unmanaged"}, nil
	}
	if err != nil {
		return Result{}, err
	}
	out := Result{BindingID: id, OperationID: j.OperationID, Status: j.Status}
	if j.Status == "restored" {
		return out, nil
	}
	b, exists, err := readConfig(filepath.Dir(j.Path), j.Path)
	if err != nil {
		return out, err
	}
	if j.ReplacementFingerprint != "" && fingerprint(b, exists) == j.ReplacementFingerprint {
		return out, nil
	}
	paths := [][]string{}
	for _, x := range j.Edits {
		paths = append(paths, x.Path)
	}
	values, err := e.editor(j.Path).Values(b, paths)
	if err != nil {
		return out, err
	}
	for i, x := range j.Edits {
		if j.Status == "prepared" || j.Status == "conflict" {
			// Original or written values are both explainable after a crash
			// or partial restore. Only third values constitute a conflict.
			if !equal(values[i], x.Before) && !equal(values[i], x.After) && !previousValue(j, x.Path, values[i]) {
				out.Status = "conflict"
				out.Conflicts = append(out.Conflicts, x.Path)
			}
		} else if !equal(values[i], x.After) {
			out.Status = "overridden"
			out.Conflicts = append(out.Conflicts, x.Path)
		}
	}
	if j.Catalog != nil {
		raw, exists, err := readConfig(filepath.Dir(j.Path), j.Catalog.Path)
		if err != nil {
			return out, err
		}
		if !exists || !catalogWritten(j, raw) {
			if j.Status == "applied" {
				out.Status = "overridden"
				out.Conflicts = append(out.Conflicts, []string{"catalog", filepath.Base(j.Catalog.Path)})
			} else if exists {
				out.Status = "conflict"
				out.Conflicts = append(out.Conflicts, []string{"catalog", filepath.Base(j.Catalog.Path)})
			}
		}
	}
	return out, nil
}

func referencesProvider(v any, provider string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, v := range x {
			if k == "model_provider" && v == provider {
				return true
			}
			if referencesProvider(v, provider) {
				return true
			}
		}
	case []any:
		for _, v := range x {
			if referencesProvider(v, provider) {
				return true
			}
		}
	}
	return false
}

func (e *Engine) recoverCatalog(j journal, out *Result) error {
	a := j.Catalog
	root := filepath.Dir(j.Path)
	raw, exists, err := readConfig(root, a.Path)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if a.Existed && bytes.Equal(raw, a.Original) {
		return nil
	}
	conflict := func() { out.Conflicts = append(out.Conflicts, []string{"catalog", filepath.Base(a.Path)}) }
	if !catalogWritten(j, raw) {
		conflict()
		return nil
	}
	if a.Existed {
		return atomicWrite(root, a.Path, a.Original, fingerprint(raw, true), true)
	}
	referenced, err := fileReferences(root, j.Path, func(v any) bool { return referencesCatalog(v, a.Path) })
	if err != nil {
		return err
	}
	if referenced {
		conflict()
		return nil
	}
	// Conditional removal has the same external-editor race boundary as config
	// publication: recheck content/path immediately before unlink, no blind delete.
	current, exists, err := readConfig(root, a.Path)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if !catalogWritten(j, current) {
		conflict()
		return nil
	}
	if err = platform.CheckManagedPath(root, a.Path); err != nil {
		return err
	}
	if err = os.Remove(a.Path); err != nil {
		return err
	}
	return platform.SyncManagedDir(root)
}

func supportedConfig(path string) bool {
	switch filepath.Base(path) {
	case "config.toml", "settings.json":
		return true
	case "cordis.patch.yml":
		profile := filepath.Base(filepath.Dir(path))
		return (profile == "web" || profile == "desktop") && filepath.Base(filepath.Dir(filepath.Dir(path))) == "profiles"
	}
	return false
}
func (e *Engine) editor(path string) Editor {
	switch filepath.Base(path) {
	case "settings.json":
		return StructuredEditor{}
	case "cordis.patch.yml":
		return StructuredEditor{DSH: true}
	}
	return e.Editor
}
