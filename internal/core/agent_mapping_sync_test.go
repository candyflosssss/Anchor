package core

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"reflect"

	"github.com/candyflosssss/Anchor/internal/modeldev"
	"gopkg.in/yaml.v3"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

func syncTestAgents(t *testing.T, f *fixture) map[string]string {
	t.Helper()
	paths := map[string]string{}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, adapter := range []string{"codex", "claude", "dsh", "dsh-desktop"} {
		root := t.TempDir()
		if err := os.Chmod(root, 0700); err != nil {
			t.Fatal(err)
		}
		path, raw := filepath.Join(root, "config.toml"), "model = \"original\"\nuser = \"keep\"\n"
		if adapter == "claude" {
			path, raw = filepath.Join(root, "settings.json"), `{"user":"keep"}`
		}
		if adapter == "dsh" || adapter == "dsh-desktop" {
			path, raw = filepath.Join(root, "profiles", map[string]string{"dsh": "web", "dsh-desktop": "desktop"}[adapter], "cordis.patch.yml"), "- id: unrelated\n  config: {user: keep}\n"
		}
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = f.store.AddBinding(state.Binding{ID: adapter, AdapterID: adapter, AdapterVersion: "1", ConfigRoot: root, Executable: exe, Model: "public", ModelScope: "all-mapped", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		if adapter == "codex" {
			path = filepath.Join(root, "anchor_codex.models.json")
		}
		paths[adapter] = path
	}
	rows, err := f.core.Agents.Start()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Status != "applied" {
			t.Fatal(rows)
		}
	}
	t.Cleanup(func() { f.core.Agents.Stop() })
	return paths
}
func syncRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func assertSyncedList(t *testing.T, paths map[string]string, ids []string, present bool) {
	t.Helper()
	for adapter, path := range paths {
		raw := syncRead(t, path)
		for _, id := range ids {
			if bytes.Contains(raw, []byte(id)) != present {
				t.Fatalf("%s model %q present != %t", adapter, id, present)
			}
		}
	}
}

func TestManagementMutationsAutomaticallySyncAgentLists(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "https://example.invalid", "synthetic")
	f.add(t, "q", "https://example.invalid", "synthetic")
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	paths := syncTestAgents(t, f)
	cl := logClient(t, f)
	ctx := t.Context()
	// Same typed client used by CLI and desktop; no explicit Agent sync call.
	if _, err := cl.ProviderAction(ctx, catalog.Action{ProviderID: "q", ExpectedRevision: 1, Action: "model-put", Target: "sol", Model: "gpt-6.1-sol", SpeedVariants: []string{"fast", "ultrafast"}}); err != nil {
		t.Fatal(err)
	}
	assertSyncedList(t, paths, []string{"sol(fast)", "sol(ultrafast)"}, true)
	// Ordinary PUT model mappings are covered as well as provider actions.
	m, err := cl.PutModel(ctx, state.Model{ID: "ordinary", Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertSyncedList(t, paths, []string{"ordinary"}, true)
	settings, err := cl.ModelSettings(ctx, catalog.ModelSettingsQuery{ModelID: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cl.SaveModelSettings(ctx, catalog.SaveModelSettings{ModelID: m.ID, ExpectedRevision: m.Revision, DisplayName: "renamed display", Configuration: settings.Configuration, ReferenceAction: "keep"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(syncRead(t, paths["codex"]), []byte("renamed display")) {
		t.Fatal("settings change not reflected in Codex catalog")
	}
	// Route-only writes must also refresh a stale catalog snapshot.
	m, err = f.store.Model("ordinary")
	if err != nil {
		t.Fatal(err)
	}
	m.DisplayName = "routing refreshed"
	m, err = f.store.PutModel(m, m.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cl.ConfigureModelRouting(ctx, catalog.ModelRouting{ModelID: m.ID, ExpectedRevision: m.Revision, Strategy: "round-robin", Order: []catalog.RouteCandidate{{ProviderID: "p", Model: "actual"}}}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(syncRead(t, paths["codex"]), []byte("routing refreshed")) {
		t.Fatal("routing mutation did not sync")
	}
	// Removing the mapping removes the exported model in all managed clients.
	m, err = f.store.Model("ordinary")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cl.ProviderAction(ctx, catalog.Action{ProviderID: "p", ExpectedRevision: 1, Action: "model-delete", Target: m.ID, ModelRevision: m.Revision, Approved: true}); err != nil {
		t.Fatal(err)
	}
	assertSyncedList(t, paths, []string{"ordinary"}, false)
	// Provider eligibility changes update the list but do not change the default.
	q, err := f.store.Provider("q")
	if err != nil {
		t.Fatal(err)
	}
	q.Enabled = false
	q, err = cl.PutProvider(ctx, q, q.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertSyncedList(t, paths, []string{"sol(fast)", "sol(ultrafast)"}, false)
	q.Enabled = true
	if _, err = cl.PutProvider(ctx, q, q.Revision, nil); err != nil {
		t.Fatal(err)
	}
	assertSyncedList(t, paths, []string{"sol(fast)", "sol(ultrafast)"}, true)
	// Provider deletion is only allowed after its model references are removed.
	for _, id := range []string{"sol", "sol(fast)", "sol(ultrafast)"} {
		mapped, err := f.store.Model(id)
		if err != nil {
			t.Fatal(err)
		}
		current, err := f.store.Provider("q")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = cl.ProviderAction(ctx, catalog.Action{ProviderID: "q", ExpectedRevision: current.Revision, Action: "model-delete", Target: id, ModelRevision: mapped.Revision, Approved: true}); err != nil {
			t.Fatal(err)
		}
	}
	q, err = f.store.Provider("q")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cl.ProviderAction(ctx, catalog.Action{ProviderID: "q", ExpectedRevision: q.Revision, Action: "delete", Approved: true}); err != nil {
		t.Fatal(err)
	}
	assertSyncedList(t, paths, []string{"sol(fast)", "sol(ultrafast)"}, false)
	// An unauthorized/failed mutation must not trigger any synchronization.
	before := map[string][]byte{}
	for a, p := range paths {
		before[a] = syncRead(t, p)
	}
	resp := controlWrite(t, f, "/control/v1/providers/p/action", catalog.Action{ProviderID: "p", ExpectedRevision: 99, Action: "model-put", Target: "denied", Model: "actual"}, true)
	if resp.StatusCode != http.StatusConflict || resp.Header.Get("X-Anchor-Agent-Sync") != "" {
		t.Fatal(resp.StatusCode, text(t, resp))
	}
	for a, p := range paths {
		if !bytes.Equal(before[a], syncRead(t, p)) {
			t.Fatal("failed mutation changed", a)
		}
	}
}

func TestAutoSyncConflictKeepsMappingAndOtherAgentsUpToDate(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "https://example.invalid", "synthetic")
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	paths := syncTestAgents(t, f)
	// A user-owned catalog edit is not overwritten. The committed mapping must
	// still return success and other bindings must still be synchronized.
	tampered := []byte(`{"models":[],"external":"preserve"}`)
	if err := os.WriteFile(paths["codex"], tampered, 0600); err != nil {
		t.Fatal(err)
	}
	resp := controlWrite(t, f, "/control/v1/providers/p/action", catalog.Action{ProviderID: "p", ExpectedRevision: 1, Action: "model-put", Target: "added", Model: "actual"}, true)
	if resp.StatusCode != 200 || resp.Header.Get("X-Anchor-Agent-Sync") != "needs_sync" {
		t.Fatal(resp.StatusCode, text(t, resp))
	}
	if _, err := f.store.Model("added"); err != nil {
		t.Fatal("mapping not committed", err)
	}
	if !bytes.Equal(tampered, syncRead(t, paths["codex"])) {
		t.Fatal("external edit overwritten")
	}
	delete(paths, "codex")
	assertSyncedList(t, paths, []string{"added"}, true)
	rows, err := f.core.Agents.Status()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.BindingID == "codex" && r.Status != "overridden" && r.Status != "needs_sync" {
			t.Fatal(r)
		}
	}
	if strings.Contains(f.logs.String(), "external") {
		t.Fatal("private config leaked in sync logs")
	}
}

func TestModelSettingsAutomaticallySyncDSHCapabilities(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "https://example.invalid", "synthetic")
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	paths := syncTestAgents(t, f)
	cl := logClient(t, f)
	ctx := t.Context()
	ptrInt := func(v int64) *int64 { return &v }
	ptrBool := func(v bool) *bool { return &v }
	ptrString := func(v string) *string { return &v }
	cfg := state.ModelConfiguration{
		Limits:             modeldev.CatalogLimits{Context: ptrInt(200000)},
		Capabilities:       modeldev.CatalogCapabilities{Vision: ptrBool(true)},
		SupportedReasoning: []modeldev.ReasoningOption{{Type: "effort", Values: []*string{ptrString("low"), ptrString("high"), ptrString("max")}}},
		DefaultReasoning:   &state.ReasoningDefault{Type: "effort", Value: ptrString("high")},
	}
	check := func(wantConfig bool) {
		t.Helper()
		for _, adapter := range []string{"dsh", "dsh-desktop"} {
			var rows []struct {
				ID     string         `yaml:"id"`
				Config map[string]any `yaml:"config"`
			}
			if err := yaml.Unmarshal(syncRead(t, paths[adapter]), &rows); err != nil {
				t.Fatal(err)
			}
			foundModels, foundDefault := false, false
			for _, row := range rows {
				if row.ID == "agent-default-model" {
					foundDefault = true
					want := any(nil)
					if wantConfig {
						want = "high"
					}
					if row.Config["reasoningEffort"] != want {
						t.Fatal(adapter, "default effort", row.Config)
					}
				}
				if row.ID != "llm-pi-ai" {
					continue
				}
				providers := row.Config["providers"].(map[string]any)
				models := providers["anchor-"+adapter].(map[string]any)["models"].([]any)
				for _, model := range models {
					entry := model.(map[string]any)
					if entry["id"] != "public" {
						continue
					}
					foundModels = true
					if wantConfig {
						if entry["contextWindow"] != 200000 || entry["name"] != "Configured name" || !reflect.DeepEqual(entry["input"], []any{"text", "image"}) || !reflect.DeepEqual(entry["reasoningEfforts"], map[string]any{"low": "low", "high": "high", "max": "max"}) {
							t.Fatal(adapter, entry)
						}
					} else {
						for _, key := range []string{"contextWindow", "input", "reasoningEfforts"} {
							if _, exists := entry[key]; exists {
								t.Fatal(adapter, "stale capability", entry)
							}
						}
					}
				}
			}
			if !foundModels || !foundDefault {
				t.Fatal(adapter, "missing plugin/model")
			}
		}
	}
	m, err := f.store.Model("public")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cl.SaveModelSettings(ctx, catalog.SaveModelSettings{ModelID: m.ID, ExpectedRevision: m.Revision, DisplayName: "Configured name", Configuration: cfg, ReferenceAction: "keep"}); err != nil {
		t.Fatal(err)
	}
	check(true)
	m, err = f.store.Model("public")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cl.SaveModelSettings(ctx, catalog.SaveModelSettings{ModelID: m.ID, ExpectedRevision: m.Revision, DisplayName: "Configured name", Configuration: state.ModelConfiguration{}, ReferenceAction: "keep"}); err != nil {
		t.Fatal(err)
	}
	check(false)
}
