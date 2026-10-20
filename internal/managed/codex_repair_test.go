package managed

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestCodexExplicitRepairKeepsConfigurationAvailable(t *testing.T) {
	for _, action := range []string{"sync", "enroll"} {
		for _, damage := range []string{"invalid", "deleted", "quoted", "multiline", "stale_markers", "legacy_journal"} {
			t.Run(action+"/"+damage, func(t *testing.T) {
				m, env, target := workspaceManager(t)
				b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", Approved: true})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := m.Start(); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(b.ConfigRoot, "config.toml")
				extra := []byte("\n# latest user settings\n[projects.\"/keep-latest\"]\ntrust_level = \"trusted\"\n")
				last := append(readSyncFile(t, path), extra...)
				if err := os.WriteFile(path, last, 0600); err != nil {
					t.Fatal(err)
				}
				if r, err := m.Refresh(b.ID, b.Revision); err != nil || r.Status != "applied" {
					t.Fatal(r, err)
				}
				before, err := m.Engine.load(b.ID)
				if err != nil {
					t.Fatal(err)
				}
				token, _ := m.Store.BindingToken(b.ID)
				raw := readSyncFile(t, path)
				switch damage {
				case "invalid", "legacy_journal":
					raw = append(raw, []byte("\nbroken = [PRIVATE-BROKEN-FIXTURE\n")...)
				case "quoted":
					raw = bytes.Replace(raw, []byte("model = \"public\""), []byte("\"model\" = \"public\""), 1)
				case "multiline":
					raw = bytes.Replace(raw, []byte("model = \"public\""), []byte("model = \"\"\"public\"\"\""), 1)
				case "stale_markers":
					provider := "anchor_" + strings.ReplaceAll(b.ID, "-", "_")
					clean, err := removeProviderSections(string(raw), provider)
					if err != nil {
						t.Fatal(err)
					}
					raw = []byte(clean + "\n# anchor:begin " + provider + "\n# anchor:end " + provider + "\n")
				}
				if damage == "legacy_journal" {
					before.LastWritten = nil
					if err := m.Engine.save(before); err != nil {
						t.Fatal(err)
					}
				}
				if damage == "deleted" {
					err = os.Remove(path)
				} else {
					err = os.WriteFile(path, raw, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				// Background work must not overwrite a user's in-progress edit.
				if _, err := m.SyncMappings(); err != nil {
					t.Fatal(err)
				}
				if damage != "deleted" && !bytes.Equal(raw, readSyncFile(t, path)) {
					t.Fatal("background sync overwrote external configuration")
				}
				workspace, err := m.Workspace(env)
				if err != nil || workspace.Targets[0].Status != "ready" || len(workspace.Models[0].AvailableTargets) == 0 {
					t.Fatal("repair action became inaccessible", workspace, err)
				}
				if action == "sync" {
					if r, err := m.Refresh(b.ID, b.Revision); err != nil || r.Status != "applied" {
						t.Fatal(r, err)
					}
				} else {
					b, err = m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", ExpectedRevision: b.Revision, Approved: true})
					if err != nil {
						t.Fatal(err)
					}
				}
				g, err := m.gateway(b)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := (Codex{}).Verify(b, g); err != nil {
					t.Fatal("repair did not produce managed configuration", err)
				}
				doc, err := document(readSyncFile(t, path))
				if err != nil || (damage != "legacy_journal" && !at(doc, []string{"projects", "/keep-latest"}).Exists) {
					t.Fatal("last valid unrelated settings lost", err)
				}
				backups, err := filepath.Glob(filepath.Join(m.Engine.Dir, b.ID+".config-*.bak"))
				if err != nil {
					t.Fatal(err)
				}
				wantBackup := damage == "invalid" || damage == "legacy_journal" || damage == "quoted" || damage == "multiline"
				if wantBackup {
					if len(backups) != 1 || !bytes.Equal(readSyncFile(t, backups[0]), raw) {
						t.Fatal("exact external file was not backed up", backups)
					}
					info, err := os.Stat(backups[0])
					if err != nil || info.Mode().Perm()&0077 != 0 {
						t.Fatal("backup exposed private config", err)
					}
				} else if len(backups) != 0 {
					t.Fatal("unnecessary full-file backup", backups)
				}
				after, err := m.Engine.load(b.ID)
				if err != nil || after.OperationID != before.OperationID || !bytes.Equal(after.Original, before.Original) {
					t.Fatal("recovery baseline replaced", err)
				}
				currentToken, _ := m.Store.BindingToken(b.ID)
				if currentToken != token {
					t.Fatal("binding credential rotated")
				}
				written, journalRaw := readSyncFile(t, path), readSyncFile(t, m.Engine.journalPath(b.ID))
				if r, err := m.Refresh(b.ID, b.Revision); err != nil || r.Status != "applied" || !bytes.Equal(written, readSyncFile(t, path)) || !bytes.Equal(journalRaw, readSyncFile(t, m.Engine.journalPath(b.ID))) {
					t.Fatal("repeated repair was not idempotent", r, err)
				}
				if r, err := m.Restore(b.ID, b.Revision); err != nil || r.Status != "restored" {
					t.Fatal("repair broke normal recovery", r, err)
				}
				if _, err := document(readSyncFile(t, path)); err != nil {
					t.Fatal("recovery reintroduced invalid TOML", err)
				}
			})
		}
	}
}

func TestCodexRepairRequiresDurableBackupAndValidRevision(t *testing.T) {
	m, env, target := workspaceManager(t)
	b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(b.ConfigRoot, "config.toml")
	raw := []byte("model = [PRIVATE-BROKEN-FIXTURE\n")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Refresh(b.ID, b.Revision+1); !errors.Is(err, state.ErrConflict) {
		t.Fatal("stale revision repaired a file", err)
	}
	if _, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", ExpectedRevision: b.Revision, Approved: false}); err == nil {
		t.Fatal("unapproved repair succeeded")
	}
	backup := filepath.Join(m.Engine.Dir, b.ID+".config-"+fingerprint(raw, true)+".bak")
	if err := os.Mkdir(backup, 0700); err != nil {
		t.Fatal(err)
	}
	before := readSyncFile(t, m.Engine.journalPath(b.ID))
	if _, err := m.Refresh(b.ID, b.Revision); err == nil {
		t.Fatal("repair ignored failed backup")
	}
	if !bytes.Equal(raw, readSyncFile(t, path)) || !bytes.Equal(before, readSyncFile(t, m.Engine.journalPath(b.ID))) {
		t.Fatal("failed backup changed configuration or journal")
	}
}

func TestCodexRepairDoesNotBypassUnsupportedOrUnapprovedTargets(t *testing.T) {
	for _, config := range []string{"model = [invalid", "profile = \"work\"\n", "[features]\nresponses_websockets = true\n"} {
		t.Run(config, func(t *testing.T) {
			m, env, target := workspaceManager(t)
			path := filepath.Join(env.CodexHome, "config.toml")
			if err := os.WriteFile(path, []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", Approved: true}); err == nil {
				t.Fatal("unmanaged broken or unsupported config silently replaced")
			}
			if string(readSyncFile(t, path)) != config {
				t.Fatal("rejected enrollment changed config")
			}
		})
	}
}
