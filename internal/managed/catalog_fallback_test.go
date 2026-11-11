package managed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func gptOnlyTemplate(t *testing.T, priority int) json.RawMessage {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(metadata(t).Entry, &entry); err != nil {
		t.Fatal(err)
	}
	entry["priority"] = priority
	entry["tool_mode"] = "code_mode_only"
	entry["supports_search_tool"] = true
	entry["service_tiers"] = []any{map[string]any{"id": "priority", "name": "Fast"}}
	entry["additional_speed_tiers"] = []any{"fast"}
	entry["availability_nux"] = map[string]any{"message": "new"}
	entry["max_context_window"] = 872000
	entry["supports_image_detail_original"] = true
	entry["model_messages"] = map[string]any{"instructions_template": "Long native prompt", "persistent_instructions": "native"}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCodexUnknownModelsCloneCachedTemplateLikeCPA(t *testing.T) {
	native := gptOnlyTemplate(t, 30)
	models := []state.Model{{ID: "gpt-alias"}, {ID: "zeta-model"}, {ID: "alpha-model", DisplayName: "Alpha"}}
	templates := map[string]json.RawMessage{"gpt-alias": native, "zeta-model": nil, "alpha-model": nil}
	raw, ids, err := buildCatalogWithFallback(models, "", templates, gptOnlyTemplate(t, 13), state.DefaultCodexSettings())
	if err != nil || len(ids) != 3 {
		t.Fatal(ids, err)
	}
	entries := catalogEntries(t, raw)
	if entries[0]["base_instructions"] == mappedBaseInstructions || entries[0]["tool_mode"] != "code_mode_only" || entries[0]["priority"] != float64(30) {
		t.Fatal("matched template changed", entries[0])
	}
	for _, entry := range entries[1:] {
		messages := entry["model_messages"].(map[string]any)
		if entry["base_instructions"] != mappedBaseInstructions || messages["instructions_template"] != mappedBaseInstructions || messages["persistent_instructions"] != nil {
			t.Fatal("cloned prompt not compacted", entry)
		}
		if entry["apply_patch_tool_type"] != nil || entry["tool_mode"] != "direct" || entry["supports_search_tool"] != false || entry["availability_nux"] != nil || entry["supports_image_detail_original"] != false || entry["use_responses_lite"] != false || entry["upgrade"] != nil {
			t.Fatal("GPT-only capability kept", entry)
		}
		if len(entry["service_tiers"].([]any)) != 0 || len(entry["additional_speed_tiers"].([]any)) != 0 {
			t.Fatal("speed tiers kept", entry)
		}
		if _, ok := entry["max_context_window"]; ok || entry["context_window"] != nil {
			t.Fatal("template context claimed for unknown model", entry)
		}
		if entry["shell_type"] != "shell_command" || entry["default_reasoning_level"] != "medium" || entry["description"] != "Anchor mapped model; upstream capabilities are not verified" {
			t.Fatal("client behaviour not cloned", entry)
		}
	}
	if entries[2]["display_name"] != "Alpha" || entries[2]["priority"] != float64(130) || entries[1]["display_name"] != "zeta-model" || entries[1]["priority"] != float64(230) {
		t.Fatal("cloned models not listed after templates alphabetically", entries[1], entries[2])
	}

	models[1].Settings = &state.ModelSettings{Configuration: state.ModelConfiguration{Limits: modeldev.CatalogLimits{Context: settingsPtr(int64(200000))}}}
	raw, _, err = buildCatalogWithFallback(models, "", templates, gptOnlyTemplate(t, 13), state.CodexSettings{OverrideModelCatalog: true, AutoCompact: true, CompactPercent: 90, Retries: 5, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if entry := catalogEntries(t, raw)[1]; entry["context_window"] != float64(200000) || entry["auto_compact_token_limit"] != float64(180000) {
		t.Fatal("consented context not projected onto cloned entry", entry)
	}
}

func TestCodexFallbackTemplateSelection(t *testing.T) {
	entry := func(priority int) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"priority": priority})
		return raw
	}
	cache := map[string]json.RawMessage{"b-model": entry(2), "a-model": entry(2), "c-model": entry(9)}
	if got := fallbackTemplate(cache); string(got) != string(cache["a-model"]) {
		t.Fatal("lowest priority with slug tie-break not chosen", string(got))
	}
	cache["gpt-5.5"] = entry(13)
	if got := fallbackTemplate(cache); string(got) != string(cache["gpt-5.5"]) {
		t.Fatal("gpt-5.5 not preferred", string(got))
	}
	if fallbackTemplate(nil) != nil {
		t.Fatal("fallback without cache")
	}
}

func TestCodexManagedCatalogClonesCacheForUnmatchedModels(t *testing.T) {
	m, env, _ := workspaceManager(t)
	if _, err := m.Store.PutModel(state.Model{ID: "claude-alias", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "claude-upstream"}}}, 0); err != nil {
		t.Fatal(err)
	}
	b, err := m.Add(state.Binding{ID: "fallback", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: env.CodexHome, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	defer m.Stop()
	byID := map[string]map[string]any{}
	for _, entry := range catalogEntries(t, readSyncFile(t, filepath.Join(env.CodexHome, catalogName(b.ID)))) {
		byID[entry["slug"].(string)] = entry
	}
	if byID["public"]["base_instructions"] == mappedBaseInstructions || byID["public"]["shell_type"] != "shell_command" {
		t.Fatal("matched model lost its native template", byID["public"])
	}
	claude := byID["claude-alias"]
	if claude["base_instructions"] != mappedBaseInstructions || claude["shell_type"] != "shell_command" || claude["context_window"] != nil {
		t.Fatal("unmatched model not cloned from the cache", claude)
	}
	if _, err := (Codex{}).Verify(b, mustGateway(t, m, b)); err != nil {
		t.Fatal(err)
	}
	// Without a usable cache the conservative entry is still exported.
	if err := os.WriteFile(filepath.Join(env.CodexHome, "models_cache.json"), []byte(`broken`), 0600); err != nil {
		t.Fatal(err)
	}
	if rows, err := m.SyncMappings(); err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	for _, entry := range catalogEntries(t, readSyncFile(t, filepath.Join(env.CodexHome, catalogName(b.ID)))) {
		if entry["base_instructions"] != mappedBaseInstructions || entry["shell_type"] != "default" {
			t.Fatal("conservative entry not used without a cache", entry)
		}
	}
}
