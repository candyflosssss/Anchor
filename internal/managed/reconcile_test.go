package managed

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func readSyncFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSyncMappingsRetainsBaselineAndDoesNotChurn(t *testing.T) {
	for _, adapter := range []string{"codex", "claude", "dsh"} {
		t.Run(adapter, func(t *testing.T) {
			m, env, _ := workspaceManager(t)
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			path, original := filepath.Join(root, "config.toml"), []byte("model = \"original\"\nuser = \"keep\"\n")
			if adapter == "claude" {
				path, original = filepath.Join(root, "settings.json"), []byte(`{"model":"original","user":"keep"}`)
			}
			if adapter == "dsh" {
				path, original = filepath.Join(root, "profiles", "web", "cordis.patch.yml"), []byte("- id: unrelated\n  config: {user: keep}\n")
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
			rows, err := m.Start()
			if err != nil || len(rows) != 1 || rows[0].Status != "applied" {
				t.Fatal(rows, err)
			}
			token, _ := m.Store.BindingToken(b.ID)
			before, err := m.Engine.load(b.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"public(fast)", "public(ultrafast)"} {
				if _, err := m.Store.PutModel(state.Model{ID: id, Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}}, 0); err != nil {
					t.Fatal(err)
				}
			}
			rows, err = m.SyncMappings()
			if err != nil || len(rows) != 1 || rows[0].Status != "applied" {
				t.Fatal(rows, err)
			}
			after, err := m.Engine.load(b.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before.Original, after.Original) || before.OperationID != after.OperationID || after.Previous != nil {
				t.Fatal("sync replaced recovery baseline")
			}
			sameToken, _ := m.Store.BindingToken(b.ID)
			if sameToken != token {
				t.Fatal("credential rotated")
			}
			list := readSyncFile(t, path)
			if adapter == "codex" {
				list = readSyncFile(t, after.Catalog.Path)
			}
			if !bytes.Contains(list, []byte("public(fast)")) || !bytes.Contains(list, []byte("public(ultrafast)")) {
				t.Fatal("speed mappings missing")
			}
			snapshot := readSyncFile(t, m.Engine.journalPath(b.ID))
			config := readSyncFile(t, path)
			if rows, err = m.SyncMappings(); err != nil || rows[0].Status != "applied" {
				t.Fatal(rows, err)
			}
			if !bytes.Equal(snapshot, readSyncFile(t, m.Engine.journalPath(b.ID))) || !bytes.Equal(config, readSyncFile(t, path)) {
				t.Fatal("no-op sync rewrote journal/config")
			}
			// Restart handoff verifies all-mapped for each adapter, without replacing
			// the baseline or mistaking non-Codex models for strict Codex metadata.
			m.PreserveForRestart()
			m2 := NewManager(m.Store)
			if rows, err = m2.Start(); err != nil || rows[0].Status != "applied" {
				t.Fatal(rows, err)
			}
			if rows, err = m2.Stop(); err != nil || rows[0].Status != "restored" {
				t.Fatal(rows, err)
			}
			restored := readSyncFile(t, path)
			if !bytes.Contains(restored, []byte("keep")) || bytes.Contains(restored, []byte("public(fast)")) || bytes.Contains(restored, []byte("anchor-")) {
				t.Fatal("original configuration not restored")
			}
			if rows, err = m2.SyncMappings(); err != nil || len(rows) != 0 {
				t.Fatal("restored binding revived", rows, err)
			}
		})
	}
}

func TestAllMappedCodexPreservesUserCatalogAndClearsOverrides(t *testing.T) {
	m, env, _ := workspaceManager(t)
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	template := metadata(t)
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(template.Entry, &entry); err != nil {
		t.Fatal(err)
	}
	entry["slug"] = json.RawMessage(`"fixture-source"`)
	source, _ := json.Marshal(map[string]any{"models": []any{entry}})
	sourcePath := filepath.Join(root, "user.models.json")
	if err := os.WriteFile(sourcePath, source, 0600); err != nil {
		t.Fatal(err)
	}
	original := []byte("model = \"old\"\nmodel_catalog_json = \"user.models.json\"\n")
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	b, err := m.Add(state.Binding{ID: "catalog", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: root, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	defer m.Stop()
	for _, id := range []string{"public(fast)", "public(ultrafast)"} {
		if _, err = m.Store.PutModel(state.Model{ID: id, Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}}, 0); err != nil {
			t.Fatal(err)
		}
	}
	if rows, err := m.SyncMappings(); err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	g, err := m.gateway(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = AdapterFor("codex").Verify(b, g); err != nil {
		t.Fatal(err)
	}
	catalog := readSyncFile(t, filepath.Join(root, catalogName(b.ID)))
	var parsed struct {
		Models []map[string]json.RawMessage `json:"models"`
	}
	if err = json.Unmarshal(catalog, &parsed); err != nil {
		t.Fatal(err)
	}
	for _, model := range parsed.Models {
		if !bytes.Equal(model["base_instructions"], entry["base_instructions"]) || !bytes.Equal(model["model_messages"], entry["model_messages"]) {
			t.Fatal("operator instructions lost")
		}
	}
	if !bytes.Equal(source, readSyncFile(t, sourcePath)) {
		t.Fatal("user catalog changed")
	}
	// Clearing an explicit display-name override must use the original source,
	// not recycle the previous generated Anchor catalog as a stale template.
	model, _ := m.Store.Model("public")
	model.DisplayName = "temporary override"
	if _, err = m.Store.PutModel(model, model.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = m.SyncMappings(); err != nil {
		t.Fatal(err)
	}
	model, _ = m.Store.Model("public")
	model.DisplayName = ""
	if _, err = m.Store.PutModel(model, model.Revision); err != nil {
		t.Fatal(err)
	}
	if rows, err := m.SyncMappings(); err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	if bytes.Contains(readSyncFile(t, filepath.Join(root, catalogName(b.ID))), []byte("temporary override")) {
		t.Fatal("cleared override retained")
	}
	if rows, err := m.Stop(); err != nil || rows[0].Status != "restored" {
		t.Fatal(rows, err)
	}
	restored := readSyncFile(t, path)
	doc, err := document(restored)
	if err != nil || doc["model_catalog_json"] != "user.models.json" {
		t.Fatal("user catalog pointer not restored", err)
	}
	if !bytes.Equal(source, readSyncFile(t, sourcePath)) {
		t.Fatal("source catalog overwritten")
	}
}

func TestReconcilePreparedRecoveryAcceptsOnlyPreviousOrNextWrites(t *testing.T) {
	for _, configNext := range []bool{false, true} {
		for _, catalogNext := range []bool{false, true} {
			t.Run(fmt.Sprintf("configNext=%t/catalogNext=%t", configNext, catalogNext), func(t *testing.T) {
				_, b, g, e := target(t, "model = \"original\"\nuser = \"keep\"\n")
				b.ModelScope = "all-mapped"
				g.Models = []state.Model{{ID: "public"}}
				p := planApply(t, b, g, e)
				previous, err := e.load(b.ID)
				if err != nil {
					t.Fatal(err)
				}
				config := readSyncFile(t, p.Path)
				g.Models = append(g.Models, state.Model{ID: "second"})
				b.Model, g.Model = "second", "second"
				if r, err := e.Reconcile(b, g); err != nil || r.Status != "applied" {
					t.Fatal(r, err)
				}
				next, err := e.load(b.ID)
				if err != nil {
					t.Fatal(err)
				}
				next.Status = "prepared"
				next.Previous = &previousWrite{Edits: previous.Edits, Catalog: previous.Catalog}
				if err = e.save(next); err != nil {
					t.Fatal(err)
				}
				if !configNext {
					if err = os.WriteFile(p.Path, config, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if !catalogNext {
					if err = os.WriteFile(previous.Catalog.Path, previous.Catalog.Written, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if r, err := e.Status(b.ID); err != nil || r.Status != "prepared" {
					t.Fatal(r, err)
				}
				if r, err := e.Recover(b.ID); err != nil || r.Status != "restored" {
					t.Fatal(r, err)
				}
				doc, err := document(readSyncFile(t, p.Path))
				if err != nil || doc["model"] != "original" || doc["user"] != "keep" {
					t.Fatal("crash recovery lost baseline", err)
				}
				if _, err = os.Stat(previous.Catalog.Path); !os.IsNotExist(err) {
					t.Fatal("catalog not removed", err)
				}
			})
		}
	}
}

func TestReconcileRejectsThirdPartyChanges(t *testing.T) {
	for _, kind := range []string{"owned-field", "catalog", "profile", "websocket"} {
		t.Run(kind, func(t *testing.T) {
			_, b, g, e := target(t, "model = \"original\"\n")
			b.ModelScope = "all-mapped"
			g.Models = []state.Model{{ID: "public"}}
			p := planApply(t, b, g, e)
			switch kind {
			case "owned-field":
				raw := readSyncFile(t, p.Path)
				raw = bytes.Replace(raw, []byte(`model = "public"`), []byte(`model = "third-party"`), 1)
				if err := os.WriteFile(p.Path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "catalog":
				if err := os.WriteFile(p.catalog.Path, []byte(`{"models":[]}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "profile":
				if err := os.WriteFile(p.Path, append([]byte("profile = \"external\"\n"), readSyncFile(t, p.Path)...), 0600); err != nil {
					t.Fatal(err)
				}
			case "websocket":
				if err := os.WriteFile(p.Path, append(readSyncFile(t, p.Path), []byte("\n[features]\nresponses_websockets = true\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := readSyncFile(t, p.Path)
			catalog := readSyncFile(t, p.catalog.Path)
			journal := readSyncFile(t, e.journalPath(b.ID))
			g.Models = append(g.Models, state.Model{ID: "second"})
			if _, err := e.Reconcile(b, g); err == nil {
				t.Fatal("third-party change accepted")
			}
			if !bytes.Equal(before, readSyncFile(t, p.Path)) || !bytes.Equal(catalog, readSyncFile(t, p.catalog.Path)) || !bytes.Equal(journal, readSyncFile(t, e.journalPath(b.ID))) {
				t.Fatal("conflicting files overwritten")
			}
		})
	}
}

func TestSyncMappingsDoesNotExpandScopeOrReviveRestoredTargets(t *testing.T) {
	m, env, _ := workspaceManager(t)
	bindings := []state.Binding{}
	snapshots := map[string][]byte{}
	for _, id := range []string{"single", "disabled", "restored"} {
		root := t.TempDir()
		if err := os.Chmod(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("model = \"old\"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		scope := "all-mapped"
		if id == "single" {
			scope = "single"
		}
		b, err := m.Add(state.Binding{ID: id, AdapterID: "codex", AdapterVersion: "1", ConfigRoot: root, Executable: env.Executable, Model: "public", ModelScope: scope, Enabled: id != "disabled"})
		if err != nil {
			t.Fatal(err)
		}
		bindings = append(bindings, b)
	}
	if _, err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if r, err := m.Engine.Recover("restored"); err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
	for _, b := range bindings {
		snapshots[b.ID] = readSyncFile(t, filepath.Join(b.ConfigRoot, "config.toml"))
	}
	if _, err := m.Store.PutModel(state.Model{ID: "added", Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}, 0); err != nil {
		t.Fatal(err)
	}
	rows, err := m.SyncMappings()
	if err != nil || len(rows) != 1 || rows[0].BindingID != "single" || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	for _, b := range bindings {
		if !bytes.Equal(snapshots[b.ID], readSyncFile(t, filepath.Join(b.ConfigRoot, "config.toml"))) {
			t.Fatal("scope/inactive configuration changed", b.ID)
		}
	}
}

func TestSyncUpgradesLegacyAllMappedJournalWithoutReplacingBaseline(t *testing.T) {
	_, b, g, e := target(t, "model = \"original\"\nmodel_catalog_json = \"existing.json\"\n")
	// Simulate the prior all-mapped implementation: it wrote only connection
	// fields, leaving the user's existing catalog pointer unmanaged.
	p := planApply(t, b, g, e)
	old, err := e.load(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	b.ModelScope = "all-mapped"
	g.Models = []state.Model{{ID: "public"}, {ID: "public(fast)"}}
	if r, err := e.Reconcile(b, g); err != nil || r.Status != "applied" {
		t.Fatal(r, err)
	}
	updated, err := e.load(b.ID)
	if err != nil || updated.Catalog == nil || !bytes.Equal(old.Original, updated.Original) {
		t.Fatal("upgrade lost baseline", err)
	}
	if r, err := e.Recover(b.ID); err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
	doc, err := document(readSyncFile(t, p.Path))
	if err != nil || doc["model_catalog_json"] != "existing.json" {
		t.Fatal("legacy pointer not restored", err)
	}
}

func TestMappedCatalogFallbackIsConservativeAndRejectsBadTemplates(t *testing.T) {
	model := state.Model{ID: "alias", Candidates: []state.Candidate{{Model: "source"}}}
	raw, ids, err := buildCatalog([]state.Model{model}, "")
	if err != nil || len(ids) != 1 || ids[0] != "alias" {
		t.Fatal(ids, err)
	}
	var catalog struct {
		Models []map[string]json.RawMessage `json:"models"`
	}
	if err = json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	entry := catalog.Models[0]
	if string(entry["context_window"]) != "null" || string(entry["supported_reasoning_levels"]) != "[]" || entry["model_messages"] != nil || entry["base_instructions"] != nil {
		t.Fatal("fallback invented capabilities/instructions")
	}
	// Optional instructions are genuinely optional in a model-name catalog.
	entry["slug"] = json.RawMessage(`"source"`)
	valid, _ := json.Marshal(entry)
	if _, _, err = buildCatalogTemplates([]state.Model{model}, "", map[string]json.RawMessage{"source": valid}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"priority", "supported_reasoning_levels", "context_window", "model_messages", "support_verbosity"} {
		original := entry[key]
		entry[key] = json.RawMessage(`{"wrong":"type"}`)
		malformed, _ := json.Marshal(entry)
		if _, _, err = buildCatalogTemplates([]state.Model{model}, "", map[string]json.RawMessage{"source": malformed}); err == nil {
			t.Fatal("malformed template accepted", key)
		}
		if original == nil {
			delete(entry, key)
		} else {
			entry[key] = original
		}
	}
}
