package managed

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func nativeCacheEntry(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(metadata(t).Entry, &entry); err != nil {
		t.Fatal(err)
	}
	delete(entry, "base_instructions")
	entry["model_messages"] = json.RawMessage(`{"instructions_template":"Synthetic native instructions: {{ personality }}", "instructions_variables":{"personality":"Synthetic personality"}}`)
	entry["shell_type"] = json.RawMessage(`"unified_exec"`)
	entry["experimental_supported_tools"] = json.RawMessage(`["synthetic_tool"]`)
	entry["input_modalities"] = json.RawMessage(`["text","image"]`)
	entry["future_client_field"] = json.RawMessage(`{"keep":true}`)
	return entry
}

func writeNativeCache(t *testing.T, root string, entry map[string]json.RawMessage) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"client_version": "0.159.3", "models": []any{entry}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "models_cache.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestMappedCachePreservesNativeInstructionsThroughRestartAndRecovery(t *testing.T) {
	m, env, _ := workspaceManager(t)
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("model = \"original\"\ndeveloper_instructions = \"Keep my language preference\"\n[features]\nkeep_feature = true\n")
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	b, err := m.Add(state.Binding{ID: "cache", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: root, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
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
	token, err := m.Store.BindingToken(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A previously generic catalog must migrate on restart, without a new
	// enrollment or adopting its generated prompt as the recovery baseline.
	entry := nativeCacheEntry(t)
	cache := writeNativeCache(t, root, entry)
	for _, id := range []string{"public(fast)", "public(ultrafast)"} {
		if _, err := m.Store.PutModel(state.Model{ID: id, Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}}, 0); err != nil {
			t.Fatal(err)
		}
	}
	m.PreserveForRestart()
	m = NewManager(m.Store)
	defer m.Stop()
	if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	assertCatalog := func() {
		t.Helper()
		var catalog struct {
			Models []map[string]json.RawMessage `json:"models"`
		}
		if err := json.Unmarshal(readSyncFile(t, before.Catalog.Path), &catalog); err != nil || len(catalog.Models) != 3 {
			t.Fatal("missing mapped models", err)
		}
		for _, model := range catalog.Models {
			for _, key := range []string{"model_messages", "shell_type", "experimental_supported_tools", "input_modalities", "truncation_policy", "future_client_field"} {
				var got, want any
				if json.Unmarshal(model[key], &got) != nil || json.Unmarshal(entry[key], &want) != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("native field lost: %s", key)
				}
			}
			if bytes.Contains(model["base_instructions"], []byte(mappedBaseInstructions)) {
				t.Fatal("native instructions replaced by generic fallback")
			}
		}
	}
	assertCatalog()
	after, err := m.Engine.load(b.ID)
	if err != nil || !bytes.Equal(before.Original, after.Original) || before.OperationID != after.OperationID {
		t.Fatal("migration changed recovery baseline", err)
	}
	if now, err := m.Store.BindingToken(b.ID); err != nil || now != token {
		t.Fatal("migration rotated token", err)
	}
	journal := readSyncFile(t, m.Engine.journalPath(b.ID))
	if rows, err := m.SyncMappings(); err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	assertCatalog()
	if !bytes.Equal(journal, readSyncFile(t, m.Engine.journalPath(b.ID))) {
		t.Fatal("no-op cache sync churned journal")
	}
	if rows, err := m.Stop(); err != nil || rows[0].Status != "restored" {
		t.Fatal(rows, err)
	}
	doc, err := document(readSyncFile(t, path))
	if err != nil || doc["model"] != "original" || doc["developer_instructions"] != "Keep my language preference" || doc["features"].(map[string]any)["keep_feature"] != true {
		t.Fatal("user configuration lost", err)
	}
	if !bytes.Equal(cache, readSyncFile(t, filepath.Join(root, "models_cache.json"))) {
		t.Fatal("native cache was rewritten")
	}
}

func TestMappedCacheSelectionAndOptionalFailures(t *testing.T) {
	for _, mode := range []string{"exact", "upstream", "unmatched", "explicit", "metadata", "invalid", "unsafe", "version", "rebuild"} {
		t.Run(mode, func(t *testing.T) {
			m, _, _ := workspaceManager(t)
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			entry := nativeCacheEntry(t)
			writeNativeCache(t, root, entry)
			model := state.Model{ID: "alias", Candidates: []state.Candidate{{Model: "fixture-source"}}}
			b := state.Binding{ID: "cache", ConfigRoot: root}
			want := true
			switch mode {
			case "exact":
				model.ID = "fixture-source"
				model.Candidates = nil
			case "unmatched":
				model.ID = "fixture-source(fast)"
				model.Candidates = []state.Candidate{{Model: "fixture-source-v2"}}
				want = false
			case "explicit":
				// An explicit upstream match wins over an exact cache alias.
				model.ID = "fixture-source"
				model.Candidates[0].Model = "operator-source"
				entry["slug"] = json.RawMessage(`"operator-source"`)
				entry["model_messages"] = json.RawMessage(`{"instructions_template":"Operator instructions"}`)
				raw, _ := json.Marshal(map[string]any{"models": []any{entry}})
				if err := os.WriteFile(filepath.Join(root, "user.json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("model_catalog_json = \"user.json\"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "metadata":
				model.Codex = metadata(t)
				want = false
			case "invalid":
				if err := os.WriteFile(filepath.Join(root, "models_cache.json"), []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
				want = false
			case "unsafe":
				cache := filepath.Join(root, "models_cache.json")
				if err := os.Rename(cache, filepath.Join(root, "other.json")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("other.json", cache); err != nil {
					t.Skip(err)
				}
				want = false
			case "version":
				b.CodexVersion = "0.100.0"
				want = false
			case "rebuild":
				if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("model = [broken"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			templates, err := m.codexTemplates(b, []state.Model{model}, mode != "rebuild")
			if err != nil || (templates[model.ID] != nil) != want {
				t.Fatal("incorrect cache selection", err)
			}
			if want {
				var selected map[string]json.RawMessage
				if err := json.Unmarshal(templates[model.ID], &selected); err != nil {
					t.Fatal("instructions did not come from selected source", err)
				}
				var got, expected any
				if json.Unmarshal(selected["model_messages"], &got) != nil || json.Unmarshal(entry["model_messages"], &expected) != nil || !reflect.DeepEqual(got, expected) {
					t.Fatal("instructions did not come from selected source")
				}
			}
		})
	}
}

func TestMappedTemplateRejectsAmbiguityAndKeepsNegativeResolution(t *testing.T) {
	model := state.Model{ID: "alias", Candidates: []state.Candidate{{Model: "one"}, {Model: "two"}}}
	templates := map[string]json.RawMessage{"one": json.RawMessage(`{}`), "two": json.RawMessage(`{}`)}
	if _, err := mappedTemplate(model, templates); err == nil {
		t.Fatal("ambiguous upstream templates accepted")
	}
	templates[model.ID] = json.RawMessage(`{"exact":true}`)
	if got, err := mappedTemplate(model, templates); err != nil || !bytes.Equal(got, templates[model.ID]) {
		t.Fatal("exact alias did not win", err)
	}
	templates[model.ID] = nil
	if got, err := mappedTemplate(model, templates); err != nil || got != nil {
		t.Fatal("negative resolution borrowed another alias template", err)
	}

}
