package managed

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"github.com/candyflosssss/Anchor/internal/state"
)

type previousWrite struct {
	Edits   []Edit           `json:"edits"`
	Catalog *catalogArtifact `json:"catalog,omitempty"`
}

func previousValue(j journal, path []string, value Value) bool {
	if j.Previous == nil {
		return false
	}
	for _, edit := range j.Previous.Edits {
		if sameField(edit.Path, path) && equal(edit.After, value) {
			return true
		}
	}
	return false
}
func sameField(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func catalogWritten(j journal, raw []byte) bool {
	return j.Catalog != nil && (bytes.Equal(raw, j.Catalog.Written) ||
		(j.Previous != nil && j.Previous.Catalog != nil && j.Previous.Catalog.Path == j.Catalog.Path && bytes.Equal(raw, j.Previous.Catalog.Written)))
}

// Reconcile changes an applied snapshot without ever publishing the restored
// user configuration. Original recovery values and scoped credentials survive.
// A durable previous/next journal makes either side of a crash recoverable.
func (e *Engine) Reconcile(b state.Binding, g GatewayBinding) (Result, error) {
	out := Result{BindingID: b.ID, Status: "needs_sync"}
	err := e.withLock(func() error {
		j, err := e.load(b.ID)
		if err != nil {
			return err
		}
		out.OperationID = j.OperationID
		if j.Status != "applied" || j.Previous != nil {
			return withDiagnostic(ErrChanged, "sync_precondition", "unfinished_journal", "")
		}
		root := filepath.Dir(j.Path)
		raw, exists, err := readConfig(root, j.Path)
		if err != nil {
			return err
		}
		inverse := make([]Edit, len(j.Edits))
		for i, edit := range j.Edits {
			inverse[i] = Edit{Path: edit.Path, Before: edit.After, After: edit.Before}
		}
		// Patch verifies every owned field. Third-party changes are never absorbed
		// as a new baseline or overwritten, even if unrelated fields may be kept.
		original, err := e.editor(j.Path).Patch(raw, inverse)
		if err != nil {
			return withDiagnostic(err, "sync_verify", "", "")
		}
		owned := ""
		if j.Catalog != nil {
			owned = j.Catalog.Path
			current, present, err := readConfig(root, owned)
			if err != nil {
				return err
			}
			if !present || !bytes.Equal(current, j.Catalog.Written) {
				return withDiagnostic(ErrChanged, "sync_verify", "catalog_mismatch", "model_catalog_json")
			}
		}
		planner, ok := AdapterFor(b.AdapterID).(interface {
			planFrom(state.Binding, GatewayBinding, []byte, bool, string) (Plan, error)
		})
		if !ok {
			return ErrUnsafe
		}
		p, err := planner.planFrom(b, g, original, exists, owned)
		if err != nil {
			return err
		}
		if p.Path != j.Path || (j.Catalog != nil && (p.catalog == nil || p.catalog.Path != j.Catalog.Path)) {
			return ErrUnsafe
		}
		// Sync never silently stops owning a field. Scope changes use their own
		// explicitly approved recovery/apply flow instead.
		for _, old := range j.Edits {
			found := false
			for _, next := range p.Edits {
				if sameField(old.Path, next.Path) {
					if !equal(old.Before, next.Before) {
						return ErrUnsafe
					}
					found = true
					break
				}
			}
			if !found {
				return ErrUnsafe
			}
		}
		next, err := e.editor(j.Path).Patch(original, p.Edits)
		if err != nil {
			return err
		}
		sameCatalog := (j.Catalog == nil && p.catalog == nil) || (j.Catalog != nil && p.catalog != nil && j.Catalog.Path == p.catalog.Path && bytes.Equal(j.Catalog.Written, p.catalog.Written))
		if bytes.Equal(raw, next) && sameCatalog {
			out.Status = "applied"
			return nil
		}
		oldCatalog := j.Catalog
		j.Previous = &previousWrite{Edits: j.Edits, Catalog: oldCatalog}
		j.Edits, j.Catalog, j.Status = p.Edits, p.catalog, "prepared"
		if err := e.save(j); err != nil {
			return err
		}
		out.Status = "recovery_pending"
		if j.Catalog != nil && !sameCatalog {
			expected := fingerprint(nil, false)
			if oldCatalog != nil {
				expected = fingerprint(oldCatalog.Written, true)
			}
			if err := atomicWrite(root, j.Catalog.Path, j.Catalog.Written, expected, true); err != nil {
				return err
			}
		}
		if !bytes.Equal(raw, next) {
			if err := atomicWrite(root, j.Path, next, fingerprint(raw, exists), true); err != nil {
				return err
			}
		}
		j.Status, j.Previous = "applied", nil
		if err := e.save(j); err != nil {
			return err
		}
		out.Status = "applied"
		return nil
	})
	return out, err
}

// SyncMappings is called after a committed management mutation. It only
// refreshes enabled, already-applied targets; it never enrolls new targets,
// expands model_scope, replaces credentials or revives restored bindings.
func (m *Manager) SyncMappings() ([]Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Result{}
	if !m.ready || m.stopping {
		return out, nil
	}
	bindings, err := m.Store.Bindings()
	if err != nil {
		return out, err
	}
	for _, b := range bindings {
		if !b.Enabled {
			continue
		}
		j, err := m.Engine.load(b.ID)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err == nil && j.Status == "restored" {
			continue
		}
		r := Result{BindingID: b.ID, Status: "needs_sync"}
		if err == nil {
			var g GatewayBinding
			g, err = m.gateway(b)
			if err == nil {
				r, err = m.Engine.Reconcile(b, g)
			}
		}
		if err != nil {
			if r.Status != "recovery_pending" {
				r.Status = "needs_sync"
			}
			m.last[b.ID] = r
		} else {
			delete(m.last, b.ID)
		}
		out = append(out, r)
	}
	return out, nil
}
