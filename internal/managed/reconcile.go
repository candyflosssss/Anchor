package managed

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"

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
	return e.reconcile(b, g, false)
}

// Reapply is an explicit user request to reclaim the already-approved fields.
// Unlike background reconciliation, it retains changed values as their next
// recovery baseline. Unchanged fields keep their original recovery values.
func (e *Engine) Reapply(b state.Binding, g GatewayBinding) (Result, error) {
	return e.reconcile(b, g, true)
}

func (e *Engine) reconcile(b state.Binding, g GatewayBinding, reclaim bool) (Result, error) {
	out := Result{BindingID: b.ID, Status: "needs_sync"}
	err := e.withLock(func() error {
		j, err := e.load(b.ID)
		if err != nil {
			return err
		}
		out.OperationID = j.OperationID
		if (j.Status != "applied" && !(reclaim && j.Status == "conflict")) || j.Previous != nil {
			return withDiagnostic(ErrChanged, "sync_precondition", "unfinished_journal", "")
		}
		root := filepath.Dir(j.Path)
		raw, exists, err := readConfig(root, j.Path)
		if err != nil {
			return err
		}
		paths := make([][]string, len(j.Edits))
		for i, edit := range j.Edits {
			paths[i] = edit.Path
		}
		values, err := e.editor(j.Path).Values(raw, paths)
		if err != nil {
			return err
		}
		inverse := make([]Edit, len(j.Edits))
		baseline := make([]Value, len(j.Edits))
		for i, edit := range j.Edits {
			inverse[i] = Edit{Path: edit.Path, Before: edit.After, After: edit.Before}
			if edit.PlanningBefore != nil {
				inverse[i].After = *edit.PlanningBefore
			}
			baseline[i] = edit.Before
			if reclaim {
				inverse[i].Before = values[i]
				if !equal(values[i], edit.After) && !equal(values[i], edit.Before) {
					baseline[i] = values[i]
				}
			}
		}
		// Restore only in memory for planning; never publish an intermediate
		// user configuration or adopt our own written values as originals.
		original, err := e.editor(j.Path).Patch(raw, inverse)
		if err != nil {
			return withDiagnostic(err, "sync_verify", "", "")
		}
		owned := ""
		var catalogRaw []byte
		var catalogPresent bool
		if j.Catalog != nil {
			owned = j.Catalog.Path
			catalogRaw, catalogPresent, err = readConfig(root, owned)
			if err != nil {
				return err
			}
			if !reclaim && (!catalogPresent || !bytes.Equal(catalogRaw, j.Catalog.Written)) {
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
		for i, old := range j.Edits {
			found := false
			for k, next := range p.Edits {
				if sameField(old.Path, next.Path) {
					if !equal(inverse[i].After, next.Before) {
						return ErrUnsafe
					}
					p.Edits[k].Before = baseline[i]
					if !equal(baseline[i], next.Before) {
						planning := next.Before
						p.Edits[k].PlanningBefore = &planning
						p.Edits[k].After = retainExtraFields(values[i], next.After)
					}
					found = true
					break
				}
			}
			if !found {
				return ErrUnsafe
			}
		}
		// Journal Before values describe recovery, not this write's precondition.
		// The actual patch and final atomic rename compare the current snapshot.
		writeEdits := append([]Edit(nil), p.Edits...)
		writePaths := make([][]string, len(writeEdits))
		for i, edit := range writeEdits {
			writePaths[i] = edit.Path
		}
		current, err := e.editor(j.Path).Values(raw, writePaths)
		if err != nil {
			return err
		}
		for i := range writeEdits {
			writeEdits[i].Before = current[i]
		}
		next, err := e.editor(j.Path).Patch(raw, writeEdits)
		if err != nil {
			return err
		}
		if j.Catalog != nil && p.catalog != nil {
			p.catalog.Original, p.catalog.Existed = j.Catalog.Original, j.Catalog.Existed
			if reclaim && (!catalogPresent || !bytes.Equal(catalogRaw, j.Catalog.Written)) {
				p.catalog.Original, p.catalog.Existed = catalogRaw, catalogPresent
			}
		}
		sameCatalog := p.catalog == nil || (catalogPresent && bytes.Equal(catalogRaw, p.catalog.Written))
		sameBaseline := reflect.DeepEqual(j.Edits, p.Edits)
		if j.Catalog != nil && p.catalog != nil {
			sameBaseline = sameBaseline && j.Catalog.Existed == p.catalog.Existed && bytes.Equal(j.Catalog.Original, p.catalog.Original)
		}
		if bytes.Equal(raw, next) && sameCatalog && sameBaseline && j.Status == "applied" {
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
			expected := fingerprint(catalogRaw, catalogPresent)
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

// A generated provider may contain third-party extension keys. Reassert only
// generated keys, retaining other nested settings instead of replacing a table.
func retainExtraFields(current, desired Value) Value {
	old, ok := current.Data.(map[string]any)
	next, nextOK := desired.Data.(map[string]any)
	if !current.Exists || !desired.Exists || !ok || !nextOK {
		return desired
	}
	merged := make(map[string]any, len(old)+len(next))
	for key, value := range old {
		merged[key] = value
	}
	for key, value := range next {
		previous, exists := old[key]
		merged[key] = retainExtraFields(Value{Exists: exists, Data: previous}, Value{Exists: true, Data: value}).Data
	}
	return Value{Exists: true, Data: merged}
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
