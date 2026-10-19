package managed

import (
	"github.com/candyflosssss/Anchor/internal/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnifiedCodexRetriesSyncAndPreserveExternalEdits(t *testing.T) {
	m, env, _ := workspaceManager(t)
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.toml")
	original := []byte("model = \"original\"\nuser = \"keep\"\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	b, err := m.Add(state.Binding{ID: "codex", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: root, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Start(); err != nil {
		t.Fatal(err)
	}
	check := func(n int) {
		t.Helper()
		raw := readSyncFile(t, path)
		doc, err := document(raw)
		if err != nil {
			t.Fatal(err)
		}
		providers := doc["model_providers"].(map[string]any)
		provider := providers["anchor_codex"].(map[string]any)
		for _, field := range []string{"request_max_retries", "stream_max_retries"} {
			if provider[field] != int64(n) {
				t.Fatal(field, provider[field], n)
			}
		}
	}
	check(5)
	out, err := m.CodexSettings()
	if err != nil || out.Settings.Retries != 5 || out.Settings.Revision != 1 {
		t.Fatal(out, err)
	}
	for _, n := range []int{0, 2, 10} {
		out, err = m.SaveCodexSettings(state.SaveCodexSettings{Retries: &n, ExpectedRevision: out.Settings.Revision})
		if err != nil || len(out.NeedsSync) != 0 {
			t.Fatal(out, err)
		}
		check(n)
	}
	// Drift is preserved by automatic sync and made visible with a saved preference.
	raw := readSyncFile(t, path)
	raw = []byte(strings.Replace(string(raw), "request_max_retries = 10", "request_max_retries = 9", 1))
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	n := 4
	out, err = m.SaveCodexSettings(state.SaveCodexSettings{Retries: &n, ExpectedRevision: out.Settings.Revision})
	if err != nil || out.Settings.Retries != 4 || len(out.NeedsSync) != 1 || out.NeedsSync[0] != b.ID {
		t.Fatal(out, err)
	}
	if string(readSyncFile(t, path)) != string(raw) {
		t.Fatal("external edit overwritten")
	}
	if _, err = m.Refresh(b.ID, b.Revision); err != nil {
		t.Fatal(err)
	}
	check(4)
	// Explicit sync retains the external provider as the recovery baseline.
	if _, err = m.Restore(b.ID, b.Revision); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readSyncFile(t, path)), "request_max_retries = 9") {
		t.Fatal("external recovery baseline lost")
	}
}

func TestCodexRetryUpgradeAndAgentScope(t *testing.T) {
	m, env, _ := workspaceManager(t)
	roots := map[string]string{}
	var codex state.Binding
	for _, adapter := range []string{"codex", "claude"} {
		root := t.TempDir()
		if err := os.Chmod(root, 0700); err != nil {
			t.Fatal(err)
		}
		if adapter == "codex" {
			if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("model = \"original\"\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		b, err := m.Add(state.Binding{ID: adapter, AdapterID: adapter, AdapterVersion: "1", ConfigRoot: root, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		roots[adapter] = root
		if adapter == "codex" {
			codex = b
		}
	}
	// Simulate the provider snapshot written by the old release (native retries=0).
	m.ready = true
	g, err := m.gateway(codex)
	if err != nil {
		t.Fatal(err)
	}
	zero := 0
	g.CodexRetries = &zero
	plan, err := (Codex{}).Plan(codex, g)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Engine.Apply(plan); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Start(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(roots["codex"], "config.toml")
	if !strings.Contains(string(readSyncFile(t, path)), "stream_max_retries = 5") {
		t.Fatal("startup did not migrate old zero")
	}
	claudePath := filepath.Join(roots["claude"], "settings.json")
	original := readSyncFile(t, claudePath)
	n := 7
	out, err := m.SaveCodexSettings(state.SaveCodexSettings{Retries: &n, ExpectedRevision: 1})
	if err != nil || len(out.NeedsSync) != 0 {
		t.Fatal(out, err)
	}
	if string(readSyncFile(t, claudePath)) != string(original) {
		t.Fatal("Codex setting changed another agent")
	}
	if !strings.Contains(string(readSyncFile(t, path)), "stream_max_retries = 7") {
		t.Fatal("Codex setting not synchronized")
	}
	if _, _, err = m.SetEnabled(codex.ID, codex.Revision, false); err != nil {
		t.Fatal(err)
	}
	// A disabled target stays restored when the global setting changes.
	restored := readSyncFile(t, path)
	n = 2
	if _, err = m.SaveCodexSettings(state.SaveCodexSettings{Retries: &n, ExpectedRevision: out.Settings.Revision}); err != nil {
		t.Fatal(err)
	}
	if string(readSyncFile(t, path)) != string(restored) {
		t.Fatal("disabled target revived")
	}
}
