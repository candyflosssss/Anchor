package managed

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestCodexCatalogDefaultDoesNotReplaceInstructions(t *testing.T) {
	for _, scope := range []string{"single", "all-compatible", "all-mapped"} {
		t.Run(scope, func(t *testing.T) {
			_, b, g, engine := catalogTarget(t, "model = \"old\"\nmodel_catalog_json = \"user.json\"\nmodel_instructions_file = \"instructions.md\"\ndeveloper_instructions = \"我的指令\"\npersonality = \"friendly\"\nmodel_context_window = 42000\nmodel_auto_compact_token_limit = 38000\n")
			b.ModelScope = scope
			// The production default must be safe even with complete imported metadata.
			g.CodexSettings = nil
			original := readSyncFile(t, filepath.Join(b.ConfigRoot, "config.toml"))
			p := planApply(t, b, g, engine)
			if p.catalog != nil || p.CatalogPath != "" {
				t.Fatal("default published a prompt-bearing catalog")
			}
			if _, err := os.Stat(filepath.Join(b.ConfigRoot, catalogName(b.ID))); !os.IsNotExist(err) {
				t.Fatal("unexpected catalog", err)
			}
			doc, err := document(readSyncFile(t, p.Path))
			if err != nil {
				t.Fatal(err)
			}
			before, _ := document(original)
			for _, key := range []string{"model_catalog_json", "model_instructions_file", "developer_instructions", "personality", "model_context_window", "model_auto_compact_token_limit"} {
				if doc[key] != before[key] {
					t.Fatalf("changed %s", key)
				}
			}
			if _, err := (Codex{}).Verify(b, g); err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Recover(b.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCodexCatalogConsentToggleAndLegacyStartup(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "toggle", true: "legacy-startup"}[legacy], func(t *testing.T) {
			m, env, _ := workspaceManager(t)
			root := env.CodexHome
			original := []byte("model = \"old\"\nmodel_catalog_json = \"user.json\"\nmodel_instructions_file = \"instructions.md\"\ndeveloper_instructions = \"保留我的提示词\"\nmodel_context_window = 42000\nmodel_auto_compact_token_limit = 38000\n")
			if err := os.WriteFile(filepath.Join(root, "config.toml"), original, 0600); err != nil {
				t.Fatal(err)
			}
			userCatalog := []byte(`{"models":[]}`)
			if err := os.WriteFile(filepath.Join(root, "user.json"), userCatalog, 0600); err != nil {
				t.Fatal(err)
			}
			b, err := m.Add(state.Binding{ID: "consent", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: root, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
				t.Fatal(rows, err)
			}
			before, err := m.Engine.load(b.ID)
			if err != nil {
				t.Fatal(err)
			}
			token, _ := m.Store.BindingToken(b.ID)
			if legacy {
				// Simulate a pre-setting installation: old settings contain no consent.
				m.PreserveForRestart()
				db, err := sql.Open("sqlite", filepath.Join(m.Store.Dir, "state.db"))
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.Exec(`UPDATE meta SET value='{"retries":5,"auto_compact":true,"compact_percent":90,"revision":1}' WHERE key='codex_settings'`)
				db.Close()
				if err != nil {
					t.Fatal(err)
				}
				m = NewManager(m.Store)
				if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
					t.Fatal(rows, err)
				}
			} else {
				off := false
				out, err := m.SaveCodexSettings(state.SaveCodexSettings{OverrideModelCatalog: &off, ExpectedRevision: 1})
				if err != nil || len(out.NeedsSync) != 0 {
					t.Fatal(out, err)
				}
			}
			defer m.Stop()
			checkNative := func() {
				t.Helper()
				doc, err := document(readSyncFile(t, filepath.Join(root, "config.toml")))
				if err != nil {
					t.Fatal(err)
				}
				if doc["model_catalog_json"] != "user.json" || doc["model_instructions_file"] != "instructions.md" || doc["developer_instructions"] != "保留我的提示词" || doc["model_context_window"] != int64(42000) || doc["model_auto_compact_token_limit"] != int64(38000) {
					t.Fatal("original instruction settings not restored")
				}
				if !bytes.Equal(userCatalog, readSyncFile(t, filepath.Join(root, "user.json"))) {
					t.Fatal("user catalog changed")
				}
			}
			checkNative()
			after, err := m.Engine.load(b.ID)
			if err != nil || !bytes.Equal(before.Original, after.Original) {
				t.Fatal("recovery baseline changed", err)
			}
			if now, _ := m.Store.BindingToken(b.ID); now != token {
				t.Fatal("token rotated")
			}
			snapshot := readSyncFile(t, m.Engine.journalPath(b.ID))
			if rows, err := m.SyncMappings(); err != nil || rows[0].Status != "applied" {
				t.Fatal(rows, err)
			}
			if !bytes.Equal(snapshot, readSyncFile(t, m.Engine.journalPath(b.ID))) {
				t.Fatal("no-op native sync changed journal")
			}
			out, err := m.CodexSettings()
			if err != nil || out.Settings.OverrideModelCatalog || out.Compaction[0].Status != "disabled" {
				t.Fatal(out, err)
			}
			on := true
			out, err = m.SaveCodexSettings(state.SaveCodexSettings{OverrideModelCatalog: &on, ExpectedRevision: out.Settings.Revision})
			if err != nil || len(out.NeedsSync) != 0 {
				t.Fatal(out, err)
			}
			doc, _ := document(readSyncFile(t, filepath.Join(root, "config.toml")))
			if doc["model_catalog_json"] != filepath.Join(root, catalogName(b.ID)) {
				t.Fatal("explicit opt-in not applied")
			}
			off := false
			out, err = m.SaveCodexSettings(state.SaveCodexSettings{OverrideModelCatalog: &off, ExpectedRevision: out.Settings.Revision})
			if err != nil || len(out.NeedsSync) != 0 {
				t.Fatal(out, err)
			}
			checkNative()
			if rows, err := m.Stop(); err != nil || rows[0].Status != "restored" {
				t.Fatal(rows, err)
			}
			if _, err := os.Stat(filepath.Join(root, catalogName(b.ID))); !os.IsNotExist(err) {
				t.Fatal("retired artifact not cleaned", err)
			}
		})
	}
}

func TestCodexCatalogOptOutPreservesExternalChanges(t *testing.T) {
	m, env, _ := workspaceManager(t)
	b, err := m.Add(state.Binding{ID: "conflict", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: env.CodexHome, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	path := filepath.Join(env.CodexHome, "config.toml")
	raw := readSyncFile(t, path)
	changed := bytes.Replace(raw, []byte(filepath.Join(env.CodexHome, catalogName(b.ID))), []byte("external.json"), 1)
	if bytes.Equal(raw, changed) {
		t.Fatal("fixture pointer not found")
	}
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	off := false
	out, err := m.SaveCodexSettings(state.SaveCodexSettings{OverrideModelCatalog: &off, ExpectedRevision: 1})
	if err != nil || len(out.NeedsSync) != 1 || out.Settings.OverrideModelCatalog {
		t.Fatal(out, err)
	}
	if !bytes.Equal(changed, readSyncFile(t, path)) {
		t.Fatal("external pointer overwritten")
	}
}

func TestCodexOptOutRemovesGeneratedPointerWithoutReadingTemplates(t *testing.T) {
	m, env, _ := workspaceManager(t)
	b, err := m.Add(state.Binding{ID: "no-original-catalog", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: env.CodexHome, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	// Opt-out must work even if a previously available native cache is corrupt.
	if err := os.WriteFile(filepath.Join(env.CodexHome, "models_cache.json"), []byte(`broken`), 0600); err != nil {
		t.Fatal(err)
	}
	off := false
	out, err := m.SaveCodexSettings(state.SaveCodexSettings{OverrideModelCatalog: &off, ExpectedRevision: 1})
	if err != nil || len(out.NeedsSync) != 0 {
		t.Fatal(out, err)
	}
	doc, err := document(readSyncFile(t, filepath.Join(env.CodexHome, "config.toml")))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"model_catalog_json", "model_context_window", "model_auto_compact_token_limit"} {
		if _, ok := doc[key]; ok {
			t.Fatalf("unexpected managed override: %s", key)
		}
	}
	workspace, err := m.Workspace(env)
	if err != nil || workspace.Bindings[0].ID != b.ID || workspace.Bindings[0].CatalogPath != "" || workspace.Bindings[0].Status != "applied" {
		t.Fatal(workspace, err)
	}
	if rows, err := m.Stop(); err != nil || rows[0].Status != "restored" {
		t.Fatal(rows, err)
	}
}
