package managed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestCodexCompactionStatusMissingWindowThenConfigured(t *testing.T) {
	m, env, _ := workspaceManager(t)
	// No matching native cache: the context really is unknown until configured.
	if err := os.Remove(filepath.Join(env.CodexHome, "models_cache.json")); err != nil {
		t.Fatal(err)
	}
	b, err := m.Add(state.Binding{ID: "codex", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: env.CodexHome, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	known := state.Model{ID: "secondary", Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}, Settings: &state.ModelSettings{Configuration: state.ModelConfiguration{Limits: modeldev.CatalogLimits{Context: settingsPtr(int64(32000))}}}}
	if _, err = m.Store.PutModel(known, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Start(); err != nil {
		t.Fatal(err)
	}
	out, err := m.CodexSettings()
	if err != nil || len(out.NeedsSync) != 0 || len(out.Compaction) != 2 {
		t.Fatal(out, err)
	}
	row := out.Compaction[0]
	if row.Status != "missing_context" || row.ContextWindow != nil || row.TokenLimit != nil || !row.IsDefault {
		t.Fatal(row)
	}
	if other := out.Compaction[1]; other.Status != "written" || other.IsDefault || other.TokenLimit == nil || *other.TokenLimit != 28800 {
		t.Fatal(other)
	}
	// Reading a settings draft/status must not mutate models, files or revision.
	before := readSyncFile(t, filepath.Join(env.CodexHome, catalogName(b.ID)))
	model, err := m.Store.Model("public")
	if err != nil || model.Settings != nil {
		t.Fatal(model, err)
	}
	if _, err = m.CodexSettings(); err != nil {
		t.Fatal(err)
	}
	if got := readSyncFile(t, filepath.Join(env.CodexHome, catalogName(b.ID))); !reflect.DeepEqual(got, before) {
		t.Fatal("GET rewrote catalog")
	}
	persisted, _ := m.Store.Model("public")
	if !reflect.DeepEqual(model, persisted) {
		t.Fatal("GET persisted a draft")
	}
	model.Settings = &state.ModelSettings{Configuration: state.ModelConfiguration{Limits: modeldev.CatalogLimits{Context: settingsPtr(int64(272000))}}}
	if _, err = m.Store.PutModel(model, model.Revision); err != nil {
		t.Fatal(err)
	}
	// Even without a preceding background failure, GET detects stale files.
	out, err = m.CodexSettings()
	if err != nil || !reflect.DeepEqual(out.NeedsSync, []string{b.ID}) || out.Compaction[0].Status != "needs_sync" || out.Compaction[0].TokenLimit != nil {
		t.Fatal(out, err)
	}
	if _, err = m.SyncMappings(); err != nil {
		t.Fatal(err)
	}
	out, err = m.CodexSettings()
	if err != nil || len(out.NeedsSync) != 0 {
		t.Fatal(out, err)
	}
	row = out.Compaction[0]
	if row.Status != "written" || row.ContextWindow == nil || *row.ContextWindow != 272000 || row.TokenLimit == nil || *row.TokenLimit != 244800 {
		t.Fatal(row)
	}
	// The report is a verified file state, never a runtime-loaded/success claim.
	encoded, _ := json.Marshal(out)
	var decoded map[string]any
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["loaded"]; ok {
		t.Fatal("invented client reload state")
	}
}

func TestCodexCompactionStatusDetectsExternalDriftWithoutWriting(t *testing.T) {
	for _, kind := range []string{"root-override", "catalog", "deleted-catalog"} {
		t.Run(kind, func(t *testing.T) {
			m, env, _ := workspaceManager(t)
			model, _ := m.Store.Model("public")
			model.Codex = metadata(t)
			if _, err := m.Store.PutModel(model, model.Revision); err != nil {
				t.Fatal(err)
			}
			b, err := m.Add(state.Binding{ID: "codex", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: env.CodexHome, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = m.Start(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(env.CodexHome, catalogName(b.ID))
			if kind == "root-override" {
				path = filepath.Join(env.CodexHome, "config.toml")
			}
			if kind == "deleted-catalog" {
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				raw := []byte("external malformed catalog")
				if kind == "root-override" {
					raw = append([]byte("model_auto_compact_token_limit = 1000\n"), readSyncFile(t, path)...)
				}
				if err = os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, beforeErr := os.ReadFile(path)
			out, err := m.CodexSettings()
			if err != nil || !reflect.DeepEqual(out.NeedsSync, []string{b.ID}) || out.Compaction[0].Status != "needs_sync" || out.Compaction[0].TokenLimit != nil {
				t.Fatal(out, err)
			}
			after, afterErr := os.ReadFile(path)
			if !reflect.DeepEqual(before, after) || (beforeErr == nil) != (afterErr == nil) {
				t.Fatal("GET changed external state")
			}
		})
	}
}

func TestCodexCompactionStatusUnmanagedDisabledAndLegacy(t *testing.T) {
	for _, scope := range []string{"all-mapped", "single"} {
		t.Run(scope, func(t *testing.T) {
			m, env, _ := workspaceManager(t)
			b, err := m.Add(state.Binding{ID: "codex", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: env.CodexHome, Executable: env.Executable, Model: "public", ModelScope: scope, Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			out, err := m.CodexSettings()
			want := "unmanaged"
			if err != nil || len(out.Compaction) != 1 || out.Compaction[0].Status != want {
				t.Fatal(out, err)
			}
			if _, err = m.Start(); err != nil {
				t.Fatal(err)
			}
			if scope == "single" {
				legacy, err := m.CodexSettings()
				if err != nil || legacy.Compaction[0].Status != "unsupported" {
					t.Fatal(legacy, err)
				}
			}
			disabled := false
			out, err = m.SaveCodexSettings(state.SaveCodexSettings{AutoCompact: &disabled, ExpectedRevision: 1})
			if err != nil || out.Compaction[0].Status != "disabled" || out.Compaction[0].TokenLimit != nil {
				t.Fatal(out, err)
			}
			if _, _, err = m.SetEnabled(b.ID, b.Revision, false); err != nil {
				t.Fatal(err)
			}
			out, err = m.CodexSettings()
			if err != nil || len(out.Compaction) != 0 || out.Compaction == nil {
				t.Fatal(out, err)
			}
		})
	}
}
