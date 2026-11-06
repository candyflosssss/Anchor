package managed

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestCodexCompactionUsesEachDeclaredWindow(t *testing.T) {
	for _, percent := range []int{90, 80} {
		settings := state.DefaultCodexSettings()
		settings.OverrideModelCatalog = true
		settings.CompactPercent = percent
		models := []state.Model{}
		windows := []int64{272000, 1000000, 32001, math.MaxInt64}
		for i, window := range windows {
			models = append(models, state.Model{ID: []string{"small", "large", "odd", "maximum"}[i], Codex: metadata(t)})
			var entry map[string]json.RawMessage
			_ = json.Unmarshal(models[i].Codex.Entry, &entry)
			entry["context_window"], _ = json.Marshal(window)
			models[i].Codex.Entry, _ = json.Marshal(entry)
		}
		models = append(models, state.Model{ID: "unknown"})
		raw, _, err := buildCatalogWithSettings(models, "", nil, settings)
		if err != nil {
			t.Fatal(err)
		}
		var catalog struct {
			Models []struct {
				Limit *int64 `json:"auto_compact_token_limit"`
			} `json:"models"`
		}
		if err = json.Unmarshal(raw, &catalog); err != nil {
			t.Fatal(err)
		}
		for i, window := range windows {
			want := window/100*int64(percent) + window%100*int64(percent)/100
			if got := catalog.Models[i].Limit; got == nil || *got != want {
				t.Fatal(i, got, want)
			}
		}
		if catalog.Models[len(windows)].Limit != nil {
			t.Fatal("invented unknown window")
		}
	}
}

func TestCodexCompactionUsesPortableOverrideAndCanDisable(t *testing.T) {
	model := state.Model{ID: "mapped", Codex: metadata(t), Settings: &state.ModelSettings{Configuration: state.ModelConfiguration{Limits: modeldev.CatalogLimits{Context: settingsPtr(int64(272000))}}}}
	before, _ := json.Marshal(model)
	for _, enabled := range []bool{true, false} {
		settings := state.DefaultCodexSettings()
		settings.OverrideModelCatalog = true
		settings.AutoCompact = enabled
		raw, _, err := buildCatalogWithSettings([]state.Model{model}, model.Codex.Version, nil, settings)
		if err != nil {
			t.Fatal(err)
		}
		var catalog struct {
			Models []map[string]json.RawMessage `json:"models"`
		}
		if err = json.Unmarshal(raw, &catalog); err != nil {
			t.Fatal(err)
		}
		limit, exists := catalog.Models[0]["auto_compact_token_limit"]
		if enabled && string(limit) != "244800" || !enabled && exists {
			t.Fatal(string(raw))
		}
	}
	after, _ := json.Marshal(model)
	if string(before) != string(after) {
		t.Fatal("source metadata mutated")
	}
}

func TestCodexCompactionSyncDisableDriftAndRecovery(t *testing.T) {
	original := "model = \"original\"\nmodel_context_window = 500000\nmodel_auto_compact_token_limit = 450000\nuser = \"keep\"\n"
	_, b, g, engine := catalogTarget(t, original)
	plan := planApply(t, b, g, engine)
	check := func(want string) {
		t.Helper()
		var catalog struct {
			Models []map[string]json.RawMessage `json:"models"`
		}
		if err := json.Unmarshal(readSyncFile(t, plan.CatalogPath), &catalog); err != nil {
			t.Fatal(err)
		}
		if string(catalog.Models[0]["auto_compact_token_limit"]) != want {
			t.Fatal(catalog)
		}
	}
	check("28800")
	raw := readSyncFile(t, plan.Path)
	if strings.Contains(string(raw), "model_context_window") || strings.Contains(string(raw), "model_auto_compact_token_limit") {
		t.Fatal(string(raw))
	}
	settings := state.DefaultCodexSettings()
	settings.OverrideModelCatalog = true
	settings.CompactPercent = 80
	g.CodexSettings = &settings
	if _, err := engine.Reconcile(b, g); err != nil {
		t.Fatal(err)
	}
	check("25600")
	settings.AutoCompact = false
	if _, err := engine.Reconcile(b, g); err != nil {
		t.Fatal(err)
	}
	check("")
	raw = readSyncFile(t, plan.Path)
	if !strings.Contains(string(raw), "model_context_window = 500000") || !strings.Contains(string(raw), "model_auto_compact_token_limit = 450000") {
		t.Fatal(string(raw))
	}
	settings.AutoCompact = true
	if _, err := engine.Reconcile(b, g); err != nil {
		t.Fatal(err)
	}
	// Insert a root key, before any TOML table header.
	raw = []byte("model_auto_compact_token_limit = 1000\n" + string(readSyncFile(t, plan.Path)))
	if err := os.WriteFile(plan.Path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Reconcile(b, g); err == nil {
		t.Fatal("external drift overwritten")
	}
	if string(raw) != string(readSyncFile(t, plan.Path)) {
		t.Fatal("external edit lost")
	}
	if _, err := engine.Reapply(b, g); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Recover(b.ID); err != nil {
		t.Fatal(err)
	}
	restored := string(readSyncFile(t, plan.Path))
	if !strings.Contains(restored, "model_auto_compact_token_limit = 1000") || !strings.Contains(restored, "model_context_window = 500000") || !strings.Contains(restored, "user = \"keep\"") {
		t.Fatal(restored)
	}
}

func TestCodexCompactionMigratesLegacyJournal(t *testing.T) {
	original := "model = \"original\"\nmodel_context_window = 500000\nmodel_auto_compact_token_limit = 450000\n"
	_, b, g, engine := catalogTarget(t, original)
	settings := state.DefaultCodexSettings()
	settings.OverrideModelCatalog = true
	settings.AutoCompact = false
	g.CodexSettings = &settings
	p := planApply(t, b, g, engine)
	// The old journal had routing/catalog edits and no compaction field ownership.
	j, err := engine.load(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacy := j.Edits[:0]
	for _, edit := range j.Edits {
		if edit.Path[0] != "model_context_window" && edit.Path[0] != "model_auto_compact_token_limit" {
			legacy = append(legacy, edit)
		}
	}
	j.Edits = legacy
	if err = engine.save(j); err != nil {
		t.Fatal(err)
	}
	settings.AutoCompact = true
	g.CodexSettings = &settings
	if _, err = engine.Reconcile(b, g); err != nil {
		t.Fatal(err)
	}
	if _, err = (Codex{}).Verify(b, g); err != nil {
		t.Fatal(err)
	}
	if raw := string(readSyncFile(t, p.Path)); strings.Contains(raw, "model_context_window") || strings.Contains(raw, "model_auto_compact_token_limit") {
		t.Fatal(raw)
	}
	if _, err = engine.Recover(b.ID); err != nil {
		t.Fatal(err)
	}
	doc, err := document(readSyncFile(t, p.Path))
	if err != nil || doc["model_context_window"] != int64(500000) || doc["model_auto_compact_token_limit"] != int64(450000) {
		t.Fatal(doc, err)
	}
}

func TestCodexCompactionPreferenceSynchronizesManagedCatalog(t *testing.T) {
	m, env, _ := workspaceManager(t)
	model, err := m.Store.Model("public")
	if err != nil {
		t.Fatal(err)
	}
	model.Codex = metadata(t)
	if _, err = m.Store.PutModel(model, model.Revision); err != nil {
		t.Fatal(err)
	}
	b, err := m.Add(state.Binding{ID: "codex", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: env.CodexHome, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Start(); err != nil {
		t.Fatal(err)
	}
	check := func(want string) {
		t.Helper()
		raw := readSyncFile(t, filepath.Join(b.ConfigRoot, catalogName(b.ID)))
		var catalog struct {
			Models []map[string]json.RawMessage `json:"models"`
		}
		if err := json.Unmarshal(raw, &catalog); err != nil {
			t.Fatal(err)
		}
		if string(catalog.Models[0]["auto_compact_token_limit"]) != want {
			t.Fatal(string(raw))
		}
	}
	check("28800")
	percent := 80
	out, err := m.SaveCodexSettings(state.SaveCodexSettings{CompactPercent: &percent, ExpectedRevision: 1})
	if err != nil || len(out.NeedsSync) != 0 {
		t.Fatal(out, err)
	}
	check("25600")
	disabled := false
	out, err = m.SaveCodexSettings(state.SaveCodexSettings{AutoCompact: &disabled, ExpectedRevision: out.Settings.Revision})
	if err != nil || len(out.NeedsSync) != 0 {
		t.Fatal(out, err)
	}
	check("")
}
