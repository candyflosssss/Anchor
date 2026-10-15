package managed

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRestartHandoffRetainsOriginalJournalAndCredential(t *testing.T) {
	m, env, target := workspaceManager(t)
	read := func(path string) []byte {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	config := filepath.Join(env.CodexHome, "config.toml")
	original := read(config)
	b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := m.Start()
	if err != nil || len(rows) != 1 || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	paths := []string{config, filepath.Join(m.Engine.Dir, b.ID+".json"), filepath.Join(env.CodexHome, catalogName(b.ID))}
	snapshots := make([][]byte, len(paths))
	for i, p := range paths {
		snapshots[i] = read(p)
	}
	token, err := m.Store.BindingToken(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertUnchanged := func() {
		t.Helper()
		for i, p := range paths {
			if !bytes.Equal(snapshots[i], read(p)) {
				t.Fatalf("handoff changed %s", filepath.Base(p))
			}
		}
		now, err := m.Store.BindingToken(b.ID)
		if err != nil || now != token {
			t.Fatal("binding credential changed", err)
		}
	}
	m.PreserveForRestart()
	assertUnchanged()
	if _, err := m.Apply(b.ID, b.Revision, ""); err == nil {
		t.Fatal("management allowed after handoff")
	}
	// Replacement manager uses the durable journal rather than taking Anchor's
	// currently written provider as the pre-management baseline.
	next := NewManager(m.Store)
	rows, err = next.Start()
	if err != nil || len(rows) != 1 || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	assertUnchanged()
	rows, err = next.Stop()
	if err != nil || len(rows) != 1 || rows[0].Status != "restored" {
		t.Fatal(rows, err)
	}
	if !bytes.Equal(original, read(config)) {
		t.Fatal("ordinary stop did not restore original config")
	}
}

func TestRestartHandoffNeverOverwritesExternalEdit(t *testing.T) {
	m, env, target := workspaceManager(t)
	_, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Start(); err != nil {
		t.Fatal(err)
	}
	m.PreserveForRestart()
	config := filepath.Join(env.CodexHome, "config.toml")
	external := []byte("model_provider = \"external\"\nmodel = \"user-model\"\n")
	if err = os.WriteFile(config, external, 0600); err != nil {
		t.Fatal(err)
	}
	next := NewManager(m.Store)
	rows, err := next.Start()
	if err != nil || len(rows) != 1 || rows[0].Status == "applied" {
		t.Fatal(rows, err)
	}
	next.Stop()
	raw, err := os.ReadFile(config)
	if err != nil || !bytes.Equal(raw, external) {
		t.Fatal("external edit overwritten", err)
	}
}
