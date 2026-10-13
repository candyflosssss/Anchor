package managed

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func discoveryFixture(t *testing.T) Environment {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".codex")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	// Match current Codex serialization, which omits the legacy parallel flag.
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(metadata(t).Entry, &entry); err != nil {
		t.Fatal(err)
	}
	delete(entry, "supports_parallel_tool_calls")
	cache, err := json.Marshal(map[string]any{"client_version": "0.159.3", "models": []any{entry}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "models_cache.json"), cache, 0600); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Environment{Home: home, CodexHome: root, Executable: exe}
}

func TestDiscoveryBoundedAndReadOnly(t *testing.T) {
	env := discoveryFixture(t)
	root := env.CodexHome
	original := []byte("model = \"old\"\n")
	os.WriteFile(filepath.Join(root, "config.toml"), original, 0600)
	os.Mkdir(filepath.Join(env.Home, ".claude"), 0700)
	targets, err := Discover(env)
	if err != nil || len(targets) != 2 || targets[0].Status != "ready" || targets[1].Supported || targets[1].Status != "unsupported" {
		t.Fatal(targets, err)
	}
	if len(targets[0].Templates) != 1 || targets[0].CatalogVersion != "0.159.3" || targets[0].Source != "CODEX_HOME" {
		t.Fatal(targets)
	}
	now, _ := os.ReadFile(filepath.Join(root, "config.toml"))
	if !bytes.Equal(now, original) {
		t.Fatal("discovery wrote config")
	}
	for _, tc := range []struct{ raw, status string }{
		{`{}`, "catalog_version_invalid"},
		{`{"client_version":"invalid","models":[]}`, "catalog_version_invalid"},
		{`{"client_version":"0.159.3","models":[{"slug":"partial"}]}`, "catalog_metadata_invalid"},
	} {
		os.WriteFile(filepath.Join(root, "models_cache.json"), []byte(tc.raw), 0600)
		targets, err = Discover(env)
		if err != nil || targets[0].Status != tc.status {
			t.Fatal(targets, err)
		}
	}
	os.Remove(filepath.Join(root, "models_cache.json"))
	if err := os.Symlink(filepath.Join(root, "config.toml"), filepath.Join(root, "models_cache.json")); err == nil {
		targets, err = Discover(env)
		if err != nil || targets[0].Status != "catalog_unsafe" {
			t.Fatal(targets, err)
		}
	}
	if _, err := Discover(Environment{Home: "relative"}); err == nil {
		t.Fatal("relative home accepted")
	}
}

func workspaceManager(t *testing.T) (*Manager, Environment, AgentTarget) {
	t.Helper()
	s, _, _, _ := target(t, "")
	secret := "fixture-secret"
	if _, err := s.PutProvider(state.Provider{ID: "p", BaseURL: "http://127.0.0.1:1", Protocol: "responses", Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, 0, &secret); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutModel(state.Model{ID: "public", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}}, 0); err != nil {
		t.Fatal(err)
	}
	env := discoveryFixture(t)
	os.WriteFile(filepath.Join(env.CodexHome, "config.toml"), []byte("model = \"old\"\n# original\n"), 0600)
	ts, err := Discover(env)
	if err != nil {
		t.Fatal(err)
	}
	return NewManager(s), env, ts[0]
}

func TestEnrollLifecycleAndDynamicCatalog(t *testing.T) {
	m, env, target := workspaceManager(t)
	in := EnrollAgent{TargetID: target.ID, Model: "public", Approved: true}
	if _, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public"}); err == nil {
		t.Fatal("unapproved enrollment")
	}
	b, err := m.Enroll(env, in)
	if err != nil {
		t.Fatal(err)
	}
	if b.Revision != 1 {
		t.Fatal(b)
	}
	original, _ := os.ReadFile(filepath.Join(env.CodexHome, "config.toml"))
	rows, err := m.Start()
	if err != nil || len(rows) != 1 || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	token, err := m.Store.BindingToken(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.PutModel(state.Model{ID: "second", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}}, 0); err != nil {
		t.Fatal(err)
	}
	workspace, err := m.Workspace(env)
	if err != nil || workspace.Bindings[0].Status != "needs_sync" {
		t.Fatal(workspace, err)
	}
	if _, err := m.Refresh(b.ID, 0); !errors.Is(err, state.ErrConflict) {
		t.Fatal(err)
	}
	if _, err := m.Refresh(b.ID, b.Revision); err != nil {
		t.Fatal(err)
	}
	assertCatalog := func(count int) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(b.ConfigRoot, catalogName(b.ID)))
		var catalog struct {
			Models []json.RawMessage `json:"models"`
		}
		if err != nil || json.Unmarshal(raw, &catalog) != nil || len(catalog.Models) != count {
			t.Fatal(string(raw), err)
		}
	}
	assertCatalog(2)
	// A process restart refreshes changed mapping metadata without capturing
	// Anchor's written values as the user's originals.
	if _, err := m.Store.PutModel(state.Model{ID: "third", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}}, 0); err != nil {
		t.Fatal(err)
	}
	m = NewManager(m.Store)
	rows, err = m.Start()
	if err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	assertCatalog(3)
	// User exclusions override exact matching and legacy provider metadata.
	in.ExpectedRevision = b.Revision
	in.ModelTemplates = map[string]string{"second": "", "public": "fixture-source"}
	b, err = m.Enroll(env, in)
	if err != nil {
		t.Fatal(err)
	}
	assertCatalog(2)
	if _, err = m.Enroll(env, in); !errors.Is(err, state.ErrConflict) {
		t.Fatal(err)
	}
	unchanged, _ := m.Store.Model("public")
	if unchanged.Codex != nil {
		t.Fatal("provider model mutated")
	}
	sameToken, _ := m.Store.BindingToken(b.ID)
	if token != sameToken {
		t.Fatal("credential rotated")
	}
	b, _, err = m.SetEnabled(b.ID, b.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Store.BindingToken(b.ID); !errors.Is(err, state.ErrNotFound) {
		t.Fatal("disabled credential remains active", err)
	}
	now, _ := os.ReadFile(filepath.Join(b.ConfigRoot, "config.toml"))
	if !bytes.Equal(now, original) {
		t.Fatal(string(now))
	}
	// Enrollment re-enables both the document and credential index.
	in.ExpectedRevision = b.Revision
	b, err = m.Enroll(env, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Store.BindingToken(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Stop(); err != nil {
		t.Fatal(err)
	}
	now, _ = os.ReadFile(filepath.Join(b.ConfigRoot, "config.toml"))
	if !bytes.Equal(now, original) {
		t.Fatal(string(now))
	}
}

func TestStartupRefreshPreservesThirdPartyEdits(t *testing.T) {
	m, env, target := workspaceManager(t)
	b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	m.Start()
	path := filepath.Join(b.ConfigRoot, "config.toml")
	raw, _ := os.ReadFile(path)
	edited := bytes.Replace(raw, []byte(`model = "public"`), []byte(`model = "external"`), 1)
	if bytes.Equal(raw, edited) {
		t.Fatal("fixture edit failed")
	}
	os.WriteFile(path, edited, 0600)
	restarted := NewManager(m.Store)
	rows, err := restarted.Start()
	if err != nil || rows[0].Status != "overridden" {
		t.Fatal(rows, err)
	}
	now, _ := os.ReadFile(path)
	if !bytes.Equal(now, edited) {
		t.Fatal("restart changed external edits")
	}
	if _, err = restarted.Refresh(b.ID, b.Revision); err == nil {
		t.Fatal("sync accepted external owned-field edit")
	}
	now, _ = os.ReadFile(path)
	if !bytes.Contains(now, []byte(`model = "external"`)) {
		t.Fatal(string(now))
	}
}

func TestBindingTemplatesResolveWithoutProviderCoupling(t *testing.T) {
	meta := *metadata(t)
	var entry map[string]any
	json.Unmarshal(meta.Entry, &entry)
	entry["slug"] = "second-source"
	second := meta
	second.Entry, _ = json.Marshal(entry)
	b := state.Binding{CodexVersion: meta.Version, CodexCatalog: map[string]state.CodexMetadata{"fixture-source": meta, "second-source": second}}
	model := state.Model{ID: "alias", Candidates: []state.Candidate{{Model: "fixture-source"}, {Model: "second-source"}}}
	if state.BindingMetadata(b, model) != nil {
		t.Fatal("ambiguous source accepted")
	}
	b.ModelTemplates = map[string]string{"alias": "fixture-source"}
	if state.BindingMetadata(b, model) == nil {
		t.Fatal("explicit choice ignored")
	}
	b.ModelTemplates["alias"] = ""
	model.Codex = &meta
	if state.BindingMetadata(b, model) != nil {
		t.Fatal("explicit exclusion ignored")
	}
	if model.Codex != &meta {
		t.Fatal("provider metadata mutated")
	}
	b.ModelTemplates = nil
	model.Codex = nil
	model.ID = "second-source"
	if selected := state.BindingMetadata(b, model); selected == nil || !bytes.Equal(selected.Entry, second.Entry) {
		t.Fatal("exact alias lost")
	}
	model.ID = "alias"
	model.Candidates = model.Candidates[:1]
	if state.BindingMetadata(b, model) == nil {
		t.Fatal("unique upstream lost")
	}
}

func TestLocalCatalogBindingCanSwitchToSingle(t *testing.T) {
	m, env, target := workspaceManager(t)
	b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	m.Start()
	b, _, err = m.SetModels(b.ID, b.Revision, "public", "single")
	if err != nil {
		t.Fatal(err)
	}
	if b.CodexVersion != "0.159.3" {
		t.Fatal("target cache version lost", b.CodexVersion)
	}
	if r, err := m.Apply(b.ID, b.Revision, ""); err != nil || r.Status != "applied" {
		t.Fatal(r, err)
	}
	if allowed, err := m.Store.BindingAllows(b, "other"); err != nil || allowed {
		t.Fatal("single allowed other model", err)
	}
	m.Stop()
}

func TestSyncLegacySingleWithoutCatalog(t *testing.T) {
	m, env, _ := workspaceManager(t)
	b, err := m.Add(state.Binding{ID: "legacy", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: env.CodexHome, Executable: env.Executable, Model: "public", ModelScope: "single", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := m.Start(); err != nil || rows[0].Status != "applied" {
		t.Fatal(rows, err)
	}
	if result, err := m.Refresh(b.ID, b.Revision); err != nil || result.Status != "applied" {
		t.Fatal(result, err)
	}
	if _, err := m.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceAutomaticMappingAvailability(t *testing.T) {
	m, env, target := workspaceManager(t)
	secret := "fixture-secret"
	if _, err := m.Store.PutProvider(state.Provider{ID: "disabled", BaseURL: "http://127.0.0.1:1", Protocol: "responses", Enabled: false, TimeoutSeconds: 5, MaxConcurrent: 1}, 0, &secret); err != nil {
		t.Fatal(err)
	}
	shared := metadata(t)
	old := *shared
	old.Version = "0.100.0"
	for _, model := range []state.Model{
		{ID: "shared", Tools: true, Streaming: true, Codex: shared, Candidates: []state.Candidate{{ProviderID: "p", Model: "upstream-not-in-cache"}}},
		{ID: "missing", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "unmatched"}}},
		{ID: "disabled", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "disabled", Model: "fixture-source"}}},
		{ID: "no-tools", Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}},
		{ID: "no-streaming", Tools: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}},
		{ID: "wrong-version", Tools: true, Streaming: true, Codex: &old, Candidates: []state.Candidate{{ProviderID: "p", Model: "unmatched"}}},
	} {
		if _, err := m.Store.PutModel(model, 0); err != nil {
			t.Fatal(err)
		}
	}
	originalMapping, err := m.Store.Model("shared")
	if err != nil {
		t.Fatal(err)
	}
	originalJSON, err := json.Marshal(originalMapping)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(env.CodexHome, "config.toml"))
	workspace, err := m.Workspace(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range workspace.Models {
		wantAvailable := model.ID == "public" || model.ID == "shared"
		if wantAvailable {
			if len(model.AvailableTargets) != 1 || model.AvailableTargets[0] != target.ID {
				t.Fatalf("%s availability: %v", model.ID, model.AvailableTargets)
			}
		} else if model.AvailableTargets == nil || len(model.AvailableTargets) != 0 {
			t.Fatalf("%s should be unavailable: %v", model.ID, model.AvailableTargets)
		}
	}
	after, _ := os.ReadFile(filepath.Join(env.CodexHome, "config.toml"))
	if !bytes.Equal(before, after) {
		t.Fatal("workspace wrote Agent configuration")
	}
	// The model with shared metadata must enroll without any target-local choice.
	b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "shared", Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.ModelTemplates) != 0 {
		t.Fatal("automatic enrollment persisted template overrides")
	}
	allowed, err := m.Store.BindingModels(b)
	if err != nil || len(allowed) != 2 {
		t.Fatal("preview and enrollment disagree", allowed, err)
	}
	// Re-saving a legacy target drops its duplicate exclusions, while preserving
	// credentials and shared mapping metadata.
	token, err := m.Store.BindingToken(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, err = m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "public", ExpectedRevision: b.Revision, ModelTemplates: map[string]string{"shared": ""}, Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err = m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "shared", ExpectedRevision: b.Revision, Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	allowed, err = m.Store.BindingModels(b)
	if err != nil || len(allowed) != 2 || len(b.ModelTemplates) != 0 {
		t.Fatal("legacy exclusions survived automatic enrollment", allowed, err)
	}
	if current, err := m.Store.BindingToken(b.ID); err != nil || current != token {
		t.Fatal("credential changed", err)
	}
	mapped, err := m.Store.Model("shared")
	if err != nil {
		t.Fatal(err)
	}
	mappedJSON, err := json.Marshal(mapped)
	if err != nil || !bytes.Equal(mappedJSON, originalJSON) {
		t.Fatal("shared mapping changed", err)
	}
	// An invalid cache remains discoverable with empty availability lists.
	if err := os.WriteFile(filepath.Join(env.CodexHome, "models_cache.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	workspace, err = m.Workspace(env)
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range workspace.Models {
		if model.AvailableTargets == nil || len(model.AvailableTargets) != 0 {
			t.Fatalf("invalid target available for %s", model.ID)
		}
	}
}
