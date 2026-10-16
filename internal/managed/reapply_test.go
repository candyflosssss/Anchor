package managed

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestExplicitSyncReclaimsDriftAndPreservesRecoveryAcrossAdapters(t *testing.T) {
	for _, adapter := range []string{"codex", "claude", "dsh", "dsh-desktop"} {
		t.Run(adapter, func(t *testing.T) {
			m, env, _ := workspaceManager(t)
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			path, original := filepath.Join(root, "config.toml"), []byte("model = \"original\"\nuser = \"keep\"\n")
			field := []string{"model"}
			if adapter == "claude" {
				path, original = filepath.Join(root, "settings.json"), []byte(`{"model":"original","user":"keep"}`)
			} else if adapter == "dsh" || adapter == "dsh-desktop" {
				profile := map[string]string{"dsh": "web", "dsh-desktop": "desktop"}[adapter]
				path, original = filepath.Join(root, "profiles", profile, "cordis.patch.yml"), []byte("- id: unrelated\n  config: {user: keep}\n")
				field = []string{"agent-default-model"}
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			b, err := m.Add(state.Binding{ID: adapter, AdapterID: adapter, AdapterVersion: "1", ConfigRoot: root, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
				t.Fatal(rows, err)
			}
			token, _ := m.Store.BindingToken(b.ID)
			before, err := m.Engine.load(b.ID)
			if err != nil {
				t.Fatal(err)
			}
			editor := m.Engine.editor(path)
			values, err := editor.Values(readSyncFile(t, path), [][]string{field})
			if err != nil {
				t.Fatal(err)
			}
			external := Value{Exists: true, Data: "external"}
			if len(field) == 1 && field[0] == "agent-default-model" {
				external.Data = map[string]any{"id": "agent-default-model", "config": map[string]any{"provider": "external-provider", "model": "external", "user": "preserved"}}
			}
			changed, err := editor.Patch(readSyncFile(t, path), []Edit{{Path: field, Before: values[0], After: external}})
			if err != nil {
				t.Fatal(err)
			}
			if adapter == "codex" {
				// A third-party encoder drops markers and reorders sections.
				doc, err := document(changed)
				if err != nil {
					t.Fatal(err)
				}
				var encoded bytes.Buffer
				if err := toml.NewEncoder(&encoded).Encode(doc); err != nil {
					t.Fatal(err)
				}
				changed = encoded.Bytes()
			}
			if err := os.WriteFile(path, changed, 0600); err != nil {
				t.Fatal(err)
			}
			if rows, err := m.SyncMappings(); err != nil || rows[0].Status != "needs_sync" || !bytes.Equal(changed, readSyncFile(t, path)) {
				t.Fatal("background sync overwrote external configuration", rows, err)
			}
			if _, err := m.Refresh(b.ID, b.Revision+1); !errors.Is(err, state.ErrConflict) {
				t.Fatal("stale revision accepted", err)
			}
			if r, err := m.Refresh(b.ID, b.Revision); err != nil || r.Status != "applied" {
				t.Fatal(r, err)
			}
			// Repeat sync and restart must not recycle written values as originals,
			// nor change the pre-enrollment structural shape (created DSH rows).
			for i := 0; i < 2; i++ {
				if r, err := m.Refresh(b.ID, b.Revision); err != nil || r.Status != "applied" {
					t.Fatal(r, err)
				}
			}
			m = NewManager(m.Store)
			if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
				t.Fatal(rows, err)
			}
			if _, err := m.Store.PutModel(state.Model{ID: "second", Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}}, 0); err != nil {
				t.Fatal(err)
			}
			if rows, err := m.SyncMappings(); err != nil || rows[0].Status != "applied" {
				t.Fatal(rows, err)
			}
			after, err := m.Engine.load(b.ID)
			if err != nil || after.OperationID != before.OperationID || !bytes.Equal(before.Original, after.Original) {
				t.Fatal("original operation replaced", err)
			}
			if current, err := m.Store.BindingToken(b.ID); err != nil || current != token {
				t.Fatal("credential changed", err)
			}
			if r, err := m.Restore(b.ID, b.Revision); err != nil || r.Status != "restored" {
				t.Fatal(r, err)
			}
			values, err = editor.Values(readSyncFile(t, path), [][]string{field})
			if err != nil || !equal(values[0], external) {
				t.Fatal("external recovery value lost", values, err)
			}
			for _, edit := range before.Edits {
				if sameField(edit.Path, field) {
					continue
				}
				values, err := editor.Values(readSyncFile(t, path), [][]string{edit.Path})
				if err != nil || !equal(values[0], edit.Before) {
					t.Fatal("unchanged field lost original baseline", edit.Path, err)
				}
			}
		})
	}
}

func TestExplicitSyncRecoversCatalogDrift(t *testing.T) {
	for _, mode := range []string{"modified", "invalid", "deleted"} {
		t.Run(mode, func(t *testing.T) {
			m, env, target := workspaceManager(t)
			b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", Approved: true})
			if err != nil {
				t.Fatal(err)
			}
			if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
				t.Fatal(rows, err)
			}
			path := filepath.Join(b.ConfigRoot, catalogName(b.ID))
			external := []byte(`{"models":[],"external":true}`)
			if mode == "invalid" {
				external = []byte("third-party incomplete write\n")
			}
			if mode == "deleted" {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, external, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if r, err := m.Refresh(b.ID, b.Revision); err != nil || r.Status != "applied" {
				t.Fatal(r, err)
			}
			if r, err := m.Refresh(b.ID, b.Revision); err != nil || r.Status != "applied" {
				t.Fatal("repeat sync lost catalog baseline", r, err)
			}
			if rows, err := m.SyncMappings(); err != nil || rows[0].Status != "applied" {
				t.Fatal(rows, err)
			}
			if r, err := m.Restore(b.ID, b.Revision); err != nil || r.Status != "restored" {
				t.Fatal(r, err)
			}
			if mode == "deleted" {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("deleted catalog resurrected", err)
				}
			} else if !bytes.Equal(external, readSyncFile(t, path)) {
				t.Fatal("external catalog not restored")
			}
		})
	}
}

func TestExplicitSyncAfterPartialRestore(t *testing.T) {
	m, env, target := workspaceManager(t)
	b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	m.Start()
	path := filepath.Join(b.ConfigRoot, "config.toml")
	raw := bytes.Replace(readSyncFile(t, path), []byte(`model = "public"`), []byte(`model = "external"`), 1)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if r, err := m.Restore(b.ID, b.Revision); err != nil || r.Status != "conflict" {
		t.Fatal(r, err)
	}
	if r, err := m.Refresh(b.ID, b.Revision); err != nil || r.Status != "applied" {
		t.Fatal("partial restore became a dead end", r, err)
	}
	if r, err := m.Restore(b.ID, b.Revision); err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
	if !bytes.Contains(readSyncFile(t, path), []byte(`model = "external"`)) {
		t.Fatal("external model not restored")
	}
}

func TestExplicitSyncPreservesProviderExtensionsAndTheirRecovery(t *testing.T) {
	_, b, g, e := target(t, "model = \"original\"\n")
	p := planApply(t, b, g, e)
	path := []string{"model_providers", "anchor_test"}
	values, err := e.Editor.Values(readSyncFile(t, p.Path), [][]string{path})
	if err != nil {
		t.Fatal(err)
	}
	external := Value{Exists: true, Data: normalize(values[0].Data)}
	external.Data.(map[string]any)["http_headers"] = map[string]any{"X-User": "keep"}
	changed, err := e.Editor.Patch(readSyncFile(t, p.Path), []Edit{{Path: path, Before: values[0], After: external}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if r, err := e.Reapply(b, g); err != nil || r.Status != "applied" {
			t.Fatal(r, err)
		}
		if r, err := e.Reconcile(b, g); err != nil || r.Status != "applied" {
			t.Fatal(r, err)
		}
	}
	if r, err := e.Status(b.ID); err != nil || r.Status != "applied" {
		t.Fatal("provider-only edit was mistaken for no-op", r, err)
	}
	if r, err := e.Recover(b.ID); err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
	values, err = e.Editor.Values(readSyncFile(t, p.Path), [][]string{path})
	if err != nil || !equal(values[0], external) {
		t.Fatal("provider extension recovery lost", values, err)
	}
}

func TestRecoverAfterThirdPartyTOMLReserialization(t *testing.T) {
	_, b, g, e := target(t, "model = \"original\"\n[model_providers]\n[model_providers.user]\nbase_url = \"http://127.0.0.1:1\"\n")
	b.ID = "测试"
	p := planApply(t, b, g, e)
	doc, err := document(readSyncFile(t, p.Path))
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := toml.NewEncoder(&encoded).Encode(doc); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Path, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if r, err := e.Recover(b.ID); err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
	doc, err = document(readSyncFile(t, p.Path))
	if err != nil || doc["model"] != "original" {
		t.Fatal(doc, err)
	}
	providers := doc["model_providers"].(map[string]any)
	if len(providers) != 1 || providers["user"] == nil {
		t.Fatal("unrelated provider changed", providers)
	}
}

type reapplyRacingEditor struct {
	Editor
	path       string
	concurrent []byte
	patches    int
}

func (e *reapplyRacingEditor) Patch(raw []byte, edits []Edit) ([]byte, error) {
	next, err := e.Editor.Patch(raw, edits)
	e.patches++
	if err == nil && e.patches == 2 {
		err = os.WriteFile(e.path, e.concurrent, 0600)
	}
	return next, err
}

func TestExplicitSyncRejectsConcurrentEditsAndKeepsRecovery(t *testing.T) {
	_, b, g, e := target(t, "model = \"original\"\n")
	p := planApply(t, b, g, e)
	raw := readSyncFile(t, p.Path)
	external := bytes.Replace(raw, []byte(`model = "public"`), []byte(`model = "external"`), 1)
	if err := os.WriteFile(p.Path, external, 0600); err != nil {
		t.Fatal(err)
	}
	concurrent := bytes.Replace(external, []byte(`model = "external"`), []byte(`model = "concurrent"`), 1)
	e.Editor = &reapplyRacingEditor{Editor: TOMLEditor{}, path: p.Path, concurrent: concurrent}
	if r, err := e.Reapply(b, g); !errors.Is(err, ErrChanged) || r.Status != "recovery_pending" {
		t.Fatal("concurrent edit accepted", r, err)
	}
	if !bytes.Equal(concurrent, readSyncFile(t, p.Path)) {
		t.Fatal("concurrent edit overwritten")
	}
	e.Editor = TOMLEditor{}
	if r, err := e.Recover(b.ID); err != nil || r.Status != "conflict" {
		t.Fatal("prepared recovery lost conflict", r, err)
	}
	if !bytes.Contains(readSyncFile(t, p.Path), []byte(`model = "concurrent"`)) {
		t.Fatal("recovery overwrote concurrent edit")
	}
}

func TestExplicitSyncPreparedRecoveryKeepsExternalBaselines(t *testing.T) {
	for _, catalogExisted := range []bool{false, true} {
		for _, phase := range []string{"before_writes", "catalog_written", "config_written"} {
			t.Run(phase+map[bool]string{false: "_deleted_catalog", true: "_external_catalog"}[catalogExisted], func(t *testing.T) {
				m, env, target := workspaceManager(t)
				b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", Approved: true})
				if err != nil {
					t.Fatal(err)
				}
				if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
					t.Fatal(rows, err)
				}
				previous, err := m.Engine.load(b.ID)
				if err != nil {
					t.Fatal(err)
				}
				externalConfig := bytes.Replace(readSyncFile(t, previous.Path), []byte(`model = "public"`), []byte(`model = "external"`), 1)
				if !bytes.Contains(externalConfig, []byte(`model = "external"`)) {
					t.Fatal("fixture did not change model")
				}
				if err := os.WriteFile(previous.Path, externalConfig, 0600); err != nil {
					t.Fatal(err)
				}
				externalCatalog := []byte("incomplete third-party catalog\n")
				if catalogExisted {
					err = os.WriteFile(previous.Catalog.Path, externalCatalog, 0600)
				} else {
					err = os.Remove(previous.Catalog.Path)
				}
				if err != nil {
					t.Fatal(err)
				}
				if r, err := m.Refresh(b.ID, b.Revision); err != nil || r.Status != "applied" {
					t.Fatal(r, err)
				}
				next, err := m.Engine.load(b.ID)
				if err != nil {
					t.Fatal(err)
				}
				// Replay each durable point of the prepared -> catalog -> config
				// transaction, then recover using a newly constructed engine.
				if phase != "config_written" {
					if err := os.WriteFile(next.Path, externalConfig, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if phase == "before_writes" {
					if catalogExisted {
						err = os.WriteFile(next.Catalog.Path, externalCatalog, 0600)
					} else {
						err = os.Remove(next.Catalog.Path)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				next.Status = "prepared"
				next.Previous = &previousWrite{Edits: previous.Edits, Catalog: previous.Catalog}
				if err := m.Engine.save(next); err != nil {
					t.Fatal(err)
				}
				engine := NewEngine(m.Store)
				if r, err := engine.Recover(b.ID); err != nil || r.Status != "restored" {
					t.Fatal(r, err)
				}
				doc, err := document(readSyncFile(t, next.Path))
				if err != nil || doc["model"] != "external" {
					t.Fatal("external model baseline lost", err)
				}
				raw, exists, err := readConfig(b.ConfigRoot, next.Catalog.Path)
				if err != nil || exists != catalogExisted || (exists && !bytes.Equal(raw, externalCatalog)) {
					t.Fatal("external catalog baseline lost", exists, err)
				}
			})
		}
	}
}
