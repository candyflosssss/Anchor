package managed

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"github.com/candyflosssss/Anchor/internal/state"
)

// Rebuild is an explicit whole-file replacement. The discarded config and
// journal stay in protected backups; ordinary disable never restores a broken
// document. The new recovery baseline is a clean, unmanaged configuration.
func (e *Engine) validateRebuild(b state.Binding) error {
	if b.AdapterID != "codex" || b.ModelScope != "all-mapped" {
		return ErrUnsafe
	}
	path := filepath.Join(b.ConfigRoot, "config.toml")
	if _, _, err := readConfig(b.ConfigRoot, path); err != nil {
		return err
	}
	if j, err := e.load(b.ID); err == nil {
		if j.Path != path {
			return ErrUnsafe
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (e *Engine) RebuildCodex(b state.Binding, g GatewayBinding) (Result, error) {
	out := Result{BindingID: b.ID, Status: "pending"}
	err := e.withLock(func() error {
		if err := e.validateRebuild(b); err != nil {
			return err
		}
		path := filepath.Join(b.ConfigRoot, "config.toml")
		raw, exists, err := readConfig(b.ConfigRoot, path)
		if err != nil {
			return err
		}
		// Plan and validate against an empty document, never parse the damaged
		// input or import its transport/profile overrides or catalog templates.
		p, err := (Codex{}).planFrom(b, g, nil, true, filepath.Join(b.ConfigRoot, catalogName(b.ID)))
		if err != nil {
			return err
		}
		next, err := (TOMLEditor{}).Patch(nil, p.Edits)
		if err != nil {
			return err
		}
		var catalogRaw []byte
		var catalogExists bool
		if p.catalog != nil {
			catalogRaw, catalogExists, err = readConfig(b.ConfigRoot, p.catalog.Path)
			if err != nil {
				return err
			}
			p.catalog.Original, p.catalog.Existed = catalogRaw, catalogExists
		}
		if exists {
			if err := e.backupCodexConfig(journal{BindingID: b.ID}, raw); err != nil {
				return err
			}
		}
		// Keep all prior recovery evidence, including interrupted operations,
		// before establishing a new clean recovery baseline.
		if old, err := e.load(b.ID); err == nil {
			prior, err := os.ReadFile(e.journalPath(b.ID))
			if err != nil {
				return err
			}
			archive := filepath.Join(e.Dir, b.ID+".journal-"+fingerprint(prior, true)+".bak")
			if err := atomicWrite(e.Dir, archive, prior, "", false); err != nil {
				return err
			}
			if old.Catalog != nil && p.catalog != nil && bytes.Equal(catalogRaw, old.Catalog.Written) {
				p.catalog.Original, p.catalog.Existed = old.Catalog.Original, old.Catalog.Existed
			}
		}
		op, err := state.Token()
		if err != nil {
			return err
		}
		j := journal{Schema: 1, BindingID: b.ID, AdapterVersion: "1", OperationID: op[:16], Path: path,
			Existed: true, Edits: p.Edits, Status: "prepared", Catalog: p.catalog, LastWritten: next,
			ReplacementFingerprint: fingerprint(raw, exists)}
		if err := e.save(j); err != nil {
			return err
		}
		out.OperationID, out.Status = j.OperationID, "recovery_pending"
		if p.catalog != nil {
			if err := atomicWrite(b.ConfigRoot, p.catalog.Path, p.catalog.Written, fingerprint(catalogRaw, catalogExists), true); err != nil {
				return err
			}
		}
		if err := atomicWrite(b.ConfigRoot, path, next, fingerprint(raw, exists), true); err != nil {
			return err
		}
		j.Status, j.ReplacementFingerprint = "applied", ""
		if err := e.save(j); err != nil {
			return err
		}
		out.Status = "applied"
		return nil
	})
	return out, err
}
