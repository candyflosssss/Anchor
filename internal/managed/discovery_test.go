package managed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestCatalogFailureCategories(t *testing.T) {
	for _, tc := range []struct{ name, raw, status string }{
		{"missing", "", "catalog_missing"},
		{"json", "{", "catalog_invalid"},
		{"version", `{"models":[]}`, "catalog_version_invalid"},
		{"empty", `{"client_version":"0.159.3","models":[]}`, "catalog_empty"},
		{"metadata", `{"client_version":"0.159.3","models":[{"slug":"private-value"}]}`, "catalog_metadata_invalid"},
		{"bytes", strings.Repeat(" ", (4<<20)+1), "catalog_too_large"},
		{"directory", "directory", "catalog_unsafe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := discoveryFixture(t)
			path := filepath.Join(env.CodexHome, "models_cache.json")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			switch tc.name {
			case "missing":
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.WriteFile(path, []byte(tc.raw), 0600); err != nil {
					t.Fatal(err)
				}
			}
			targets, err := Discover(env)
			if err != nil || len(targets) != 1 || targets[0].Status != tc.status {
				t.Fatal(targets, err)
			}
			if targets[0].CatalogVersion != "" || len(targets[0].Templates) != 0 {
				t.Fatal("partial catalog exposed")
			}
			_, _, err = localCatalog(env.CodexHome)
			if err == nil || err.Error() != tc.status {
				t.Fatal("unsafe or incorrect diagnostic", err)
			}
		})
	}
	env := discoveryFixture(t)
	entry := metadata(t).Entry
	for _, n := range []int{2, 129} {
		models := make([]json.RawMessage, n)
		for i := range models {
			models[i] = entry
		}
		raw, _ := json.Marshal(map[string]any{"client_version": "0.159.3", "models": models})
		if err := os.WriteFile(filepath.Join(env.CodexHome, "models_cache.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := localCatalog(env.CodexHome)
		want := "catalog_duplicate"
		if n == 129 {
			want = "catalog_too_large"
		}
		if catalogStatus(err) != want {
			t.Fatal(err)
		}
	}
}

func TestCurrentCodexCacheSurvivesEnrollmentAndCatalogGeneration(t *testing.T) {
	m, env, target := workspaceManager(t)
	// Unknown context is a valid Codex option, not a reason to discard a cache.
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(metadata(t).Entry, &entry); err != nil {
		t.Fatal(err)
	}
	delete(entry, "supports_parallel_tool_calls")
	entry["context_window"] = json.RawMessage("null")
	entry["model_messages"] = json.RawMessage(`{"instructions_template":"Synthetic test template", "tool_description":"Synthetic"}`)
	delete(entry, "base_instructions")
	cache, _ := json.Marshal(map[string]any{"client_version": "0.159.3", "models": []any{entry}})
	if err := os.WriteFile(filepath.Join(env.CodexHome, "models_cache.json"), cache, 0600); err != nil {
		t.Fatal(err)
	}
	targets, err := Discover(env)
	if err != nil || targets[0].Status != "ready" {
		t.Fatal(targets, err)
	}
	binding, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	models, err := m.Store.BindingModels(binding)
	if err != nil {
		t.Fatal(err)
	}
	raw, ids, err := buildCatalog(models, binding.CodexVersion)
	if err != nil || len(ids) != 1 || ids[0] != "public" {
		t.Fatal(ids, err)
	}
	var generated struct {
		Models []map[string]json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(raw, &generated); err != nil {
		t.Fatal(err)
	}
	out := generated.Models[0]
	if _, ok := out["supports_parallel_tool_calls"]; ok {
		t.Fatal("fabricated parallel capability")
	}
	if string(out["context_window"]) != "null" {
		t.Fatal("fabricated context size")
	}
	if string(out["model_messages"]) != string(entry["model_messages"]) {
		var a, b any
		json.Unmarshal(out["model_messages"], &a)
		json.Unmarshal(entry["model_messages"], &b)
		aRaw, _ := json.Marshal(a)
		bRaw, _ := json.Marshal(b)
		if string(aRaw) != string(bRaw) {
			t.Fatal("instruction template changed")
		}
	}
	// Shared provider metadata remains untouched by importing a local cache.
	shared, err := m.Store.Model("public")
	if err != nil || shared.Codex != nil {
		t.Fatal(shared, err)
	}
	if err := (state.CodexMetadata{Version: binding.CodexVersion, Entry: mustJSON(t, out)}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestDiscoverDSHKnownRoots(t *testing.T) {
	for _, tc := range []struct{ name, override string }{
		{"default", ""}, {"override", "custom"}, {"deduplicated", ".dsh"}, {"relative-ignored", "relative"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, ".dsh")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			env := Environment{Home: home}
			if tc.override != "" {
				env.DSHHome = tc.override
				if tc.override != "relative" {
					env.DSHHome = filepath.Join(home, tc.override)
					if err := os.MkdirAll(env.DSHHome, 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			targets, err := Discover(env)
			want := 1
			if tc.name == "override" {
				want = 2
			}
			if err != nil || len(targets) != want {
				t.Fatal(targets, err)
			}
			for _, target := range targets {
				if target.Agent != "dsh" || target.Supported || target.Status != "unsupported" || len(target.Templates) != 0 {
					t.Fatal(target)
				}
			}
			if tc.name == "override" && (targets[0].Source != "DSH_HOME" || targets[0].ConfigRoot != env.DSHHome) {
				t.Fatal(targets)
			}
		})
	}
}
