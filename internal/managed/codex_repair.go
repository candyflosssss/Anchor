package managed

import (
	"bytes"
	"errors"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/candyflosssss/Anchor/internal/state"
)

// Explicit updates may normalize valid TOML that the byte-preserving editor
// cannot express. Never use this fallback for background sync or invalid TOML.
// The caller backs up the exact observed file before publishing the result.
type codexRepairEditor struct {
	Editor
	normalized bool
}

func (e *codexRepairEditor) Patch(raw []byte, edits []Edit) ([]byte, error) {
	next, err := e.Editor.Patch(raw, edits)
	if !errors.Is(err, ErrUnsafe) {
		return next, err
	}
	doc, err := document(raw)
	if err != nil {
		return nil, err
	}
	for _, edit := range edits {
		if len(edit.Path) == 0 || !equal(at(doc, edit.Path), edit.Before) {
			return nil, ErrChanged
		}
		parent := doc
		switch {
		case len(edit.Path) == 1:
			switch edit.Path[0] {
			case "model", "model_provider", "web_search", "model_catalog_json":
			default:
				return nil, ErrUnsafe
			}
		case len(edit.Path) == 2 && edit.Path[0] == "model_providers":
			var ok bool
			parent, ok = doc["model_providers"].(map[string]any)
			if !ok {
				if _, exists := doc["model_providers"]; exists {
					return nil, ErrUnsafe
				}
				parent = map[string]any{}
				doc["model_providers"] = parent
			}
		default:
			return nil, ErrUnsafe
		}
		key := edit.Path[len(edit.Path)-1]
		if edit.After.Exists {
			parent[key] = tomlNumbers(edit.After.Data)
		} else {
			delete(parent, key)
		}
	}
	var out bytes.Buffer
	if err := toml.NewEncoder(&out).Encode(doc); err != nil {
		return nil, ErrUnsafe
	}
	if len(out.Bytes()) > maxConfig {
		return nil, ErrUnsafe
	}
	actual, err := document(out.Bytes())
	if err != nil || !equal(Value{Exists: true, Data: doc}, Value{Exists: true, Data: actual}) {
		return nil, ErrUnsafe
	}
	// Emit the same owned section layout as ordinary writes so the next
	// explicit update does not move tables or rewrite a fresh snapshot again.
	reaffirm := append([]Edit(nil), edits...)
	for i := range reaffirm {
		reaffirm[i].Before = reaffirm[i].After
	}
	if stable, err := (TOMLEditor{}).Patch(out.Bytes(), reaffirm); err == nil {
		next = stable
	} else {
		next = out.Bytes()
	}
	e.normalized = true
	return next, nil
}

// Old schema1 journals have no full snapshot. Reconstruct their last write
// from the original document and the approved edits, keeping recovery separate.
func codexLastWritten(j journal) ([]byte, error) {
	if len(j.LastWritten) > 0 {
		paths := make([][]string, len(j.Edits))
		for i, edit := range j.Edits {
			paths[i] = edit.Path
		}
		values, err := (TOMLEditor{}).Values(j.LastWritten, paths)
		if err != nil {
			return nil, err
		}
		for i, value := range values {
			if !equal(value, j.Edits[i].After) {
				return nil, ErrUnsafe
			}
		}
		return j.LastWritten, nil
	}
	paths := make([][]string, len(j.Edits))
	for i, edit := range j.Edits {
		paths[i] = edit.Path
	}
	values, err := (TOMLEditor{}).Values(j.Original, paths)
	if err != nil {
		return nil, err
	}
	edits := append([]Edit(nil), j.Edits...)
	for i := range edits {
		edits[i].Before = values[i]
	}
	editor := codexRepairEditor{Editor: TOMLEditor{}}
	return editor.Patch(j.Original, edits)
}

func (e *Engine) backupCodexConfig(j journal, raw []byte) error {
	path := filepath.Join(e.Dir, j.BindingID+".config-"+fingerprint(raw, true)+".bak")
	previous, exists, err := readConfig(e.Dir, path)
	if err != nil {
		return err
	}
	if exists {
		if !bytes.Equal(previous, raw) {
			return ErrUnsafe
		}
		return nil
	}
	return atomicWrite(e.Dir, path, raw, fingerprint(nil, false), true)
}

// A broken known target remains available for explicit repair in the desktop
// workspace. Unregistered targets, unsupported transports and filesystem errors
// do not acquire permission merely because their config cannot be inspected.
func (m *Manager) codexRepairable(b state.Binding) bool {
	if b.AdapterID != "codex" {
		return false
	}
	path := filepath.Join(b.ConfigRoot, "config.toml")
	raw, exists, err := readConfig(b.ConfigRoot, path)
	if err != nil {
		return false
	}
	if _, err := document(raw); err == nil && exists {
		return false
	}
	j, err := m.Engine.load(b.ID)
	if err != nil || j.Path != path || j.Previous != nil || j.ReplacementFingerprint != "" || (j.Status != "applied" && j.Status != "conflict") {
		return false
	}
	_, err = codexLastWritten(j)
	return err == nil
}
