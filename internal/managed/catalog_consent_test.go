package managed

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func buildConsentedCatalog(models []state.Model, version string) ([]byte, []string, error) {
	settings := state.DefaultCodexSettings()
	settings.OverrideModelCatalog = true
	return buildCatalogWithSettings(models, version, nil, settings)
}

func catalogEntries(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var catalog struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	return catalog.Models
}

func TestCodexCatalogDefaultListsModelsWithoutAnchorOverrides(t *testing.T) {
	for _, scope := range []string{"single", "all-compatible", "all-mapped"} {
		t.Run(scope, func(t *testing.T) {
			_, b, g, engine := catalogTarget(t, "model = \"old\"\nmodel_catalog_json = \"user.json\"\nmodel_instructions_file = \"instructions.md\"\ndeveloper_instructions = \"我的指令\"\npersonality = \"friendly\"\nmodel_context_window = 42000\nmodel_auto_compact_token_limit = 38000\n")
			b.ModelScope = scope
			g.Models[0].Settings = &state.ModelSettings{Configuration: state.ModelConfiguration{Limits: modeldev.CatalogLimits{Context: settingsPtr(int64(272000))}, ParallelToolCalls: settingsPtr(false)}}
			// The production default must still publish the model list.
			g.CodexSettings = nil
			original := readSyncFile(t, filepath.Join(b.ConfigRoot, "config.toml"))
			p := planApply(t, b, g, engine)
			if p.catalog == nil || p.CatalogPath != filepath.Join(b.ConfigRoot, catalogName(b.ID)) || len(p.AvailableModels) != 2 {
				t.Fatal("default did not publish the model list", p.CatalogPath, p.AvailableModels)
			}
			for i, entry := range catalogEntries(t, readSyncFile(t, p.CatalogPath)) {
				if entry["slug"] != g.Models[i].ID || entry["visibility"] != "list" || entry["context_window"] != float64(32000) || entry["supports_parallel_tool_calls"] != true || !strings.Contains(entry["base_instructions"].(string), "Test fixture") {
					t.Fatal("native template not kept verbatim", entry)
				}
				if _, ok := entry["auto_compact_token_limit"]; ok {
					t.Fatal("compaction configured without consent", entry)
				}
			}
			doc, err := document(readSyncFile(t, p.Path))
			if err != nil {
				t.Fatal(err)
			}
			if doc["model_catalog_json"] != p.CatalogPath {
				t.Fatal("model list pointer not applied")
			}
			before, _ := document(original)
			for _, key := range []string{"model_instructions_file", "developer_instructions", "personality", "model_context_window", "model_auto_compact_token_limit"} {
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
			doc, _ = document(readSyncFile(t, p.Path))
			if doc["model_catalog_json"] != "user.json" {
				t.Fatal("original catalog pointer not restored")
			}
		})
	}
}

func TestCodexNameOnlyCatalogAddsAnchorFieldsOnlyWithConsent(t *testing.T) {
	model := state.Model{ID: "claude-alias", Settings: &state.ModelSettings{Configuration: state.ModelConfiguration{Limits: modeldev.CatalogLimits{Context: settingsPtr(int64(200000))}, Capabilities: modeldev.CatalogCapabilities{Vision: settingsPtr(true)}}}}
	raw, ids, err := buildCatalogWithSettings([]state.Model{model}, "", nil, state.DefaultCodexSettings())
	if err != nil || len(ids) != 1 {
		t.Fatal(ids, err)
	}
	entry := catalogEntries(t, raw)[0]
	// Codex rejects instruction-less entries; the explicit fallback keeps the list valid.
	if entry["slug"] != "claude-alias" || entry["base_instructions"] != mappedBaseInstructions || entry["context_window"] != nil || len(entry["input_modalities"].([]any)) != 1 {
		t.Fatal("Anchor capability overrides exported without consent", entry)
	}
	if _, ok := entry["auto_compact_token_limit"]; ok {
		t.Fatal("compaction configured without consent", entry)
	}
	raw, _, err = buildConsentedCatalog([]state.Model{model}, "")
	if err != nil {
		t.Fatal(err)
	}
	entry = catalogEntries(t, raw)[0]
	if entry["context_window"] != float64(200000) || entry["auto_compact_token_limit"] != float64(180000) || len(entry["input_modalities"].([]any)) != 2 {
		t.Fatal("consented overrides missing", entry)
	}
}

func TestCodexCatalogConsentKeepsModelListAndLegacyStartup(t *testing.T) {
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
			catalogPath := filepath.Join(root, catalogName(b.ID))
			checkListOnly := func() {
				t.Helper()
				doc, err := document(readSyncFile(t, filepath.Join(root, "config.toml")))
				if err != nil {
					t.Fatal(err)
				}
				if doc["model_catalog_json"] != catalogPath {
					t.Fatal("model list stopped syncing without prompt consent")
				}
				if doc["model_instructions_file"] != "instructions.md" || doc["developer_instructions"] != "保留我的提示词" || doc["model_context_window"] != int64(42000) || doc["model_auto_compact_token_limit"] != int64(38000) {
					t.Fatal("original instruction or context settings not preserved")
				}
				entries := catalogEntries(t, readSyncFile(t, catalogPath))
				if len(entries) != 1 || entries[0]["slug"] != "public" || entries[0]["base_instructions"] == mappedBaseInstructions {
					t.Fatal("list-only catalog did not keep the native template", entries)
				}
				if !bytes.Equal(userCatalog, readSyncFile(t, filepath.Join(root, "user.json"))) {
					t.Fatal("user catalog changed")
				}
			}
			checkListOnly()
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
				t.Fatal("no-op list-only sync changed journal")
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
			if _, ok := doc["model_context_window"]; doc["model_catalog_json"] != catalogPath || ok {
				t.Fatal("explicit opt-in not applied")
			}
			off := false
			out, err = m.SaveCodexSettings(state.SaveCodexSettings{OverrideModelCatalog: &off, ExpectedRevision: out.Settings.Revision})
			if err != nil || len(out.NeedsSync) != 0 {
				t.Fatal(out, err)
			}
			checkListOnly()
			if rows, err := m.Stop(); err != nil || rows[0].Status != "restored" {
				t.Fatal(rows, err)
			}
			doc, _ = document(readSyncFile(t, filepath.Join(root, "config.toml")))
			if doc["model_catalog_json"] != "user.json" {
				t.Fatal("original catalog pointer not restored")
			}
			if _, err := os.Stat(catalogPath); !os.IsNotExist(err) {
				t.Fatal("generated catalog not cleaned", err)
			}
		})
	}
}

func TestCodexRetiredCatalogIsAdoptedForModelList(t *testing.T) {
	m, env, _ := workspaceManager(t)
	b, err := m.Add(state.Binding{ID: "retired", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: env.CodexHome, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	defer m.Stop()
	// Earlier cores removed the pointer on opt-out and kept the file as a retired
	// journal artifact. The next sync must republish the list from that state.
	path := filepath.Join(env.CodexHome, "config.toml")
	raw := readSyncFile(t, path)
	catalogPath := filepath.Join(env.CodexHome, catalogName(b.ID))
	retired := bytes.Replace(raw, []byte("model_catalog_json = \""+catalogPath+"\"\n"), nil, 1)
	if bytes.Equal(raw, retired) {
		t.Fatal("fixture pointer not found")
	}
	j, err := m.Engine.load(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, edit := range j.Edits {
		if sameField(edit.Path, []string{"model_catalog_json"}) {
			j.Edits[i].After = edit.Before
		}
	}
	j.LastWritten = retired
	if err := m.Engine.save(j); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, retired, 0600); err != nil {
		t.Fatal(err)
	}
	if rows, err := m.SyncMappings(); err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	doc, err := document(readSyncFile(t, path))
	if err != nil || doc["model_catalog_json"] != catalogPath {
		t.Fatal("retired catalog not republished", err)
	}
	if _, err := (Codex{}).Verify(b, mustGateway(t, m, b)); err != nil {
		t.Fatal(err)
	}
}

func mustGateway(t *testing.T, m *Manager, b state.Binding) GatewayBinding {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.Store.Binding(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	g, err := m.gateway(current)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestCodexCatalogSettingSyncPreservesExternalChanges(t *testing.T) {
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

func TestCodexOptOutToleratesCorruptNativeCache(t *testing.T) {
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
	catalogPath := filepath.Join(env.CodexHome, catalogName(b.ID))
	doc, err := document(readSyncFile(t, filepath.Join(env.CodexHome, "config.toml")))
	if err != nil || doc["model_catalog_json"] != catalogPath {
		t.Fatal("model list pointer removed", err)
	}
	for _, key := range []string{"model_context_window", "model_auto_compact_token_limit"} {
		if _, ok := doc[key]; ok {
			t.Fatalf("unexpected managed override: %s", key)
		}
	}
	if entries := catalogEntries(t, readSyncFile(t, catalogPath)); len(entries) != 1 || entries[0]["slug"] != "public" {
		t.Fatal(entries)
	}
	workspace, err := m.Workspace(env)
	if err != nil || workspace.Bindings[0].ID != b.ID || workspace.Bindings[0].CatalogPath != catalogPath || workspace.Bindings[0].Status != "applied" {
		t.Fatal(workspace, err)
	}
	if rows, err := m.Stop(); err != nil || rows[0].Status != "restored" {
		t.Fatal(rows, err)
	}
}
