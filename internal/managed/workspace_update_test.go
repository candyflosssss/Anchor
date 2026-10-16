package managed

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestEnrollUpdatesDraftAndMappingsWhileReclaimingDrift(t *testing.T) {
	for _, adapter := range []string{"codex", "claude", "dsh", "dsh-desktop"} {
		for _, mode := range []string{"unchanged", "external", "recovery-conflict"} {
			t.Run(adapter+"/"+mode, func(t *testing.T) {
				m, env, target := workspaceManager(t)
				path, field := filepath.Join(env.CodexHome, "config.toml"), []string{"model"}
				if adapter != "codex" {
					root := t.TempDir()
					if err := os.Chmod(root, 0700); err != nil {
						t.Fatal(err)
					}
					if adapter == "claude" {
						env.ClaudeHome = root
						path = filepath.Join(root, "settings.json")
						if err := os.WriteFile(path, []byte(`{"model":"original","user":"keep"}`), 0600); err != nil {
							t.Fatal(err)
						}
					} else {
						env.DSHHome = root
						profile := map[string]string{"dsh": "web", "dsh-desktop": "desktop"}[adapter]
						path, field = filepath.Join(root, "profiles", profile, "cordis.patch.yml"), []string{"agent-default-model"}
						if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(path, []byte("- id: unrelated\n  config: {user: keep}\n"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					targets, err := Discover(env)
					if err != nil {
						t.Fatal(err)
					}
					for _, candidate := range targets {
						if candidate.Agent == adapter {
							target = candidate
						}
					}
				}
				// Codex starts with legacy all-compatible policy; the desktop update
				// must also migrate it to all-mapped without losing recovery values.
				b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", Approved: true})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := m.Start(); err != nil {
					t.Fatal(err)
				}
				token, _ := m.Store.BindingToken(b.ID)
				before, err := m.Engine.load(b.ID)
				if err != nil {
					t.Fatal(err)
				}
				editor := m.Engine.editor(path)
				external := Value{Exists: true, Data: "external"}
				if adapter == "dsh" || adapter == "dsh-desktop" {
					external.Data = map[string]any{"id": "agent-default-model", "config": map[string]any{"provider": "external", "model": "external", "user": "keep"}}
				}
				if mode != "unchanged" {
					values, err := editor.Values(readSyncFile(t, path), [][]string{field})
					if err != nil {
						t.Fatal(err)
					}
					raw, err := editor.Patch(readSyncFile(t, path), []Edit{{Path: field, Before: values[0], After: external}})
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, raw, 0600); err != nil {
						t.Fatal(err)
					}
					if mode == "recovery-conflict" {
						if r, err := m.Engine.Recover(b.ID); err != nil || r.Status != "conflict" {
							t.Fatal(r, err)
						}
					}
				}
				if _, err := m.Store.PutModel(state.Model{ID: "second", Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}}, 0); err != nil {
					t.Fatal(err)
				}
				input := EnrollAgent{TargetID: target.ID, Model: "second", ModelScope: "all-mapped", ExpectedRevision: b.Revision, Approved: true}
				raw := readSyncFile(t, path)
				stale := input
				stale.ExpectedRevision--
				if _, err := m.Enroll(env, stale); !errors.Is(err, state.ErrConflict) || !bytes.Equal(raw, readSyncFile(t, path)) {
					t.Fatal("stale update changed configuration", err)
				}
				for i := 0; i < 2; i++ {
					input.ExpectedRevision = b.Revision
					b, err = m.Enroll(env, input)
					if err != nil || b.Model != "second" || b.ModelScope != "all-mapped" || b.Revision != input.ExpectedRevision+1 {
						t.Fatal(b, err)
					}
					g, err := m.gateway(b)
					if err != nil || len(g.Models) != 2 {
						t.Fatal("latest mappings not included", g, err)
					}
					if _, err := AdapterFor(adapter).Verify(b, g); err != nil {
						t.Fatal("updated settings were not applied", err)
					}
				}
				if current, _ := m.Store.BindingToken(b.ID); current != token {
					t.Fatal("update rotated credentials")
				}
				after, err := m.Engine.load(b.ID)
				if err != nil || before.OperationID != after.OperationID || !bytes.Equal(before.Original, after.Original) {
					t.Fatal("update replaced recovery operation", err)
				}
				if r, err := m.Restore(b.ID, b.Revision); err != nil || r.Status != "restored" {
					t.Fatal(r, err)
				}
				for _, edit := range before.Edits {
					want := edit.Before
					if mode != "unchanged" && sameField(edit.Path, field) {
						want = external
					}
					values, err := editor.Values(readSyncFile(t, path), [][]string{edit.Path})
					if err != nil || !equal(values[0], want) {
						t.Fatal("recovery baseline changed", edit.Path, values, want, err)
					}
				}
			})
		}
	}
}

func TestEnrollUpdateRebuildsExternallyChangedCatalog(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "corrupt", true: "missing"}[missing], func(t *testing.T) {
			m, env, target := workspaceManager(t)
			b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", Approved: true})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.Start(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(b.ConfigRoot, catalogName(b.ID))
			external := []byte("external invalid catalog")
			if missing {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, external, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			b, err = m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", ExpectedRevision: b.Revision, Approved: true})
			if err != nil {
				t.Fatal(err)
			}
			g, err := m.gateway(b)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := (Codex{}).Verify(b, g); err != nil {
				t.Fatal(err)
			}
			if r, err := m.Restore(b.ID, b.Revision); err != nil || r.Status != "restored" {
				t.Fatal(r, err)
			}
			raw, err := os.ReadFile(path)
			if missing && !errors.Is(err, os.ErrNotExist) || !missing && (err != nil || !bytes.Equal(raw, external)) {
				t.Fatal("external catalog recovery baseline lost", string(raw), err)
			}
		})
	}
}
