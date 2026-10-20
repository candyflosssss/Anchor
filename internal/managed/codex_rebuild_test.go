package managed

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestCodexRebuildCleanConfiguration(t *testing.T) {
	for _, known := range []bool{false, true} {
		for _, input := range []string{"model = [broken", "profile = \"work\"\n[features]\nresponses_websockets = true\n", "model = \"other\"\n[projects.keep]\ntrust_level = \"trusted\"\n"} {
			t.Run(input+map[bool]string{true: "/known", false: "/new"}[known], func(t *testing.T) {
				m, env, target := workspaceManager(t)
				if _, err := m.Start(); err != nil {
					t.Fatal(err)
				}
				var b state.Binding
				var err error
				var token string
				if known {
					b, err = m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", Approved: true})
					if err != nil {
						t.Fatal(err)
					}
					token, _ = m.Store.BindingToken(b.ID)
				}
				path := filepath.Join(env.CodexHome, "config.toml")
				if err := os.WriteFile(path, []byte(input), 0600); err != nil {
					t.Fatal(err)
				}
				workspace, err := m.Workspace(env)
				if err != nil || len(workspace.Models[0].AvailableTargets) == 0 {
					t.Fatal("rebuild inaccessible", workspace, err)
				}
				b, err = m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", ExpectedRevision: b.Revision, Approved: true, Rebuild: true})
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
				doc, err := document(readSyncFile(t, path))
				if err != nil || doc["profile"] != nil || doc["projects"] != nil || doc["features"] != nil {
					t.Fatal("not a clean configuration", err)
				}
				backup := filepath.Join(m.Engine.Dir, b.ID+".config-"+fingerprint([]byte(input), true)+".bak")
				if !bytes.Equal(readSyncFile(t, backup), []byte(input)) {
					t.Fatal("backup differs")
				}
				if known {
					current, _ := m.Store.BindingToken(b.ID)
					if current != token {
						t.Fatal("credential rotated")
					}
					archives, _ := filepath.Glob(filepath.Join(m.Engine.Dir, b.ID+".journal-*.bak"))
					if len(archives) != 1 {
						t.Fatal("old journal not archived")
					}
				}
				if r, err := m.Refresh(b.ID, b.Revision); err != nil || r.Status != "applied" {
					t.Fatal(r, err)
				}
				if r, err := m.Restore(b.ID, b.Revision); err != nil || r.Status != "restored" {
					t.Fatal(r, err)
				}
				if _, err := document(readSyncFile(t, path)); err != nil {
					t.Fatal("restored invalid config", err)
				}
			})
		}
	}
}

func TestCodexRebuildRejectsStaleApprovalAndBackupFailure(t *testing.T) {
	m, env, target := workspaceManager(t)
	m.Start()
	path := filepath.Join(env.CodexHome, "config.toml")
	raw := []byte("model = [broken")
	os.WriteFile(path, raw, 0600)
	in := EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", Approved: true, Rebuild: true}
	in.ExpectedRevision = 1
	if _, err := m.Enroll(env, in); !errors.Is(err, state.ErrConflict) {
		t.Fatal(err)
	}
	in.ExpectedRevision, in.Approved = 0, false
	if _, err := m.Enroll(env, in); err == nil {
		t.Fatal("unapproved replacement")
	}
	in.Approved = true
	os.MkdirAll(filepath.Join(m.Engine.Dir, target.ID+".config-"+fingerprint(raw, true)+".bak"), 0700)
	if _, err := m.Enroll(env, in); err == nil {
		t.Fatal("ignored backup failure")
	}
	if !bytes.Equal(raw, readSyncFile(t, path)) {
		t.Fatal("failed backup replaced original")
	}
}

func TestCodexRebuildInterruptedBeforeConfigReplacement(t *testing.T) {
	m, env, target := workspaceManager(t)
	m.Start()
	b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", Approved: true, Rebuild: true})
	if err != nil {
		t.Fatal(err)
	}
	j, err := m.Engine.load(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("model = [broken")
	j.Status, j.ReplacementFingerprint = "prepared", fingerprint(raw, true)
	if err := m.Engine.save(j); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(env.CodexHome, "config.toml")
	os.WriteFile(path, raw, 0600)
	if r, err := m.Engine.Status(b.ID); err != nil || r.Status != "prepared" {
		t.Fatal(r, err)
	}
	if r, err := m.Engine.Recover(b.ID); err != nil || r.Status != "conflict" {
		t.Fatal(r, err)
	}
	if !bytes.Equal(raw, readSyncFile(t, path)) {
		t.Fatal("recovery modified an unpublished config")
	}
	// Explicit rebuild is also the way forward from an interrupted operation.
	if _, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ModelScope: "all-mapped", Approved: true, Rebuild: true, ExpectedRevision: b.Revision}); err != nil {
		t.Fatal(err)
	}
}
