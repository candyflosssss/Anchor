package desktop

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestSharedAgentContract(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/agents.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Contract  string         `json:"contract"`
		Workspace AgentWorkspace `json:"workspace"`
		Enroll    EnrollAgent    `json:"enroll"`
		Sync      ManageAgent    `json:"sync"`
		Update    EnrollAgent    `json:"update"`
		Rebuild   EnrollAgent    `json:"rebuild"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if !fixture.Rebuild.Rebuild || !fixture.Rebuild.Approved || fixture.Rebuild.ModelScope != "all-mapped" || fixture.Contract != Contract || len(fixture.Workspace.Targets) != 1 || len(fixture.Workspace.Models) != 1 || len(fixture.Workspace.Bindings) != 1 || !fixture.Enroll.Approved || fixture.Sync.Action != "sync" || !fixture.Update.Approved || fixture.Update.ExpectedRevision != fixture.Workspace.Bindings[0].Revision || fixture.Update.ModelScope != "all-mapped" {
		t.Fatal("contract drift")
	}
	encoded, _ := json.Marshal(fixture.Workspace)
	var original map[string]json.RawMessage
	json.Unmarshal(raw, &original)
	var a, b any
	json.Unmarshal(original["workspace"], &a)
	json.Unmarshal(encoded, &b)
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	if string(aa) != string(bb) {
		t.Fatal("DTO fields drift")
	}
}

func TestRestrictedDesktopTokenInvocation(t *testing.T) {
	if !IsAgentTokenInvocation([]string{"--data-dir", "/fixture", "agent", "token", "codex-fixture"}) {
		t.Fatal("helper rejected")
	}
	for _, args := range [][]string{{"agent", "token", "x"}, {"--data-dir", "/fixture", "agent", "list", "x"}, {"--data-dir", "/fixture", "agent", "token", "../x"}, {"--data-dir", "/fixture", "agent", "token", "x", "--extra"}, {"--data-dir", "", "agent", "token", "x"}} {
		if IsAgentTokenInvocation(args) {
			t.Fatal("broader invocation accepted", args)
		}
	}
}

func TestRealCoreAgentBridgeLifecycle(t *testing.T) {
	// Only synthetic home/config/cache; no personal agent files are accessed.
	home := t.TempDir()
	root := filepath.Join(home, ".codex")
	os.Mkdir(root, 0700)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", root)
	raw, err := os.ReadFile("../state/testdata/codex-models.json")
	if err != nil {
		t.Fatal(err)
	}
	var cache map[string]any
	if err = json.Unmarshal(raw, &cache); err != nil {
		t.Fatal(err)
	}
	cache["client_version"] = "0.159.3"
	raw, _ = json.Marshal(cache)
	os.WriteFile(filepath.Join(root, "models_cache.json"), raw, 0600)
	os.WriteFile(filepath.Join(root, "config.toml"), []byte("model = \"original\"\n"), 0600)
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	st, err := state.Open(s.options.Dir, false)
	if err != nil {
		t.Fatal(err)
	}
	secret := "synthetic"
	if _, err = st.PutProvider(state.Provider{ID: "p", BaseURL: "http://127.0.0.1:1", Protocol: "responses", Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, 0, &secret); err != nil {
		t.Fatal(err)
	}
	if _, err = st.PutModel(state.Model{ID: "public", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}}, 0); err != nil {
		t.Fatal(err)
	}
	st.Close()
	r := b.AgentWorkspace()
	if r.Problem != nil || r.Data == nil || len(r.Data.Targets) != 1 || r.Data.Targets[0].Status != "ready" {
		t.Fatalf("discovery: %+v", r)
	}
	if len(r.Data.Models) != 1 || len(r.Data.Models[0].AvailableTargets) != 1 || r.Data.Models[0].AvailableTargets[0] != r.Data.Targets[0].ID {
		t.Fatal("core availability missing at desktop boundary", r.Data.Models)
	}
	r = b.EnrollAgent(EnrollAgent{TargetID: r.Data.Targets[0].ID, Model: "public", Approved: true})
	if r.Problem != nil || r.Data.Bindings[0].Status != "applied" {
		t.Fatalf("enroll: %+v", r)
	}
	row := r.Data.Bindings[0]
	if len(row.ModelTemplates) != 0 {
		t.Fatal("automatic enrollment stored template overrides")
	}
	exe, _ := os.Executable()
	helper := exec.Command(exe, "--data-dir", s.options.Dir, "agent", "token", row.ID)
	tokenRaw, helperErr := helper.Output()
	if helperErr != nil || strings.TrimSpace(string(tokenRaw)) == "" {
		t.Fatal("credential helper failed", helperErr)
	}
	r = b.ManageAgent(ManageAgent{BindingID: row.ID, ExpectedRevision: row.Revision, Action: "sync"})
	if r.Problem != nil {
		t.Fatal(r.Problem)
	}
	r = b.ManageAgent(ManageAgent{BindingID: row.ID, ExpectedRevision: row.Revision, Action: "disable"})
	if r.Problem != nil || r.Data.Bindings[0].Enabled {
		t.Fatalf("disable: %+v", r)
	}
	now, _ := os.ReadFile(filepath.Join(root, "config.toml"))
	if string(now) != "model = \"original\"\n" {
		t.Fatal(string(now))
	}
	row = r.Data.Bindings[0]
	r = b.ManageAgent(ManageAgent{BindingID: row.ID, ExpectedRevision: row.Revision, Action: "enable"})
	if r.Problem != nil || r.Data.Bindings[0].Status != "applied" {
		t.Fatalf("enable: %+v", r)
	}
	row = r.Data.Bindings[0]
	r = b.ManageAgent(ManageAgent{BindingID: row.ID, ExpectedRevision: row.Revision - 1, Action: "sync"})
	if r.Problem == nil || r.Problem.Code != "conflict" {
		t.Fatalf("revision conflict: %+v", r)
	}
	path := filepath.Join(root, "config.toml")
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), `model = "public"`, `model = "external-edit"`, 1))
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	r = b.ManageAgent(ManageAgent{BindingID: row.ID, ExpectedRevision: row.Revision, Action: "sync"})
	if r.Problem != nil || r.Data.Bindings[0].Status != "applied" || r.Data.Bindings[0].Revision != row.Revision {
		t.Fatalf("explicit sync did not reclaim external edit: %+v", r)
	}
	now, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(now), `model = "public"`) {
		t.Fatal("explicit sync did not apply default model", err)
	}
	// The unified desktop update must submit the draft model and reclaim drift
	// through one EnrollAgent call instead of applying the previously saved model.
	st, err = state.Open(s.options.Dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.PutModel(state.Model{ID: "second", Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}}, 0); err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(now), `model = "public"`, `model = "external-edit"`, 1))
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	update := EnrollAgent{TargetID: r.Data.Targets[0].ID, Model: "second", ModelScope: "all-mapped", ExpectedRevision: row.Revision, Approved: true}
	r = b.EnrollAgent(update)
	if r.Problem != nil || r.Data.Bindings[0].Model != "second" || r.Data.Bindings[0].Status != "applied" || r.Data.Bindings[0].Revision != row.Revision+1 || len(r.Data.Bindings[0].AvailableModels) != 2 {
		t.Fatalf("unified update: %+v", r)
	}
	row = r.Data.Bindings[0]
	now, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(now), `model = "second"`) {
		t.Fatal("unified update ignored draft", err)
	}
	stale := b.EnrollAgent(update)
	if stale.Problem == nil || stale.Problem.Code != "conflict" {
		t.Fatal("stale unified update accepted", stale)
	}
	r = b.ManageAgent(ManageAgent{BindingID: row.ID, ExpectedRevision: row.Revision, Action: "disable"})
	if r.Problem != nil || r.Data.Bindings[0].Enabled {
		t.Fatalf("disable after reclaim: %+v", r)
	}
	now, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(now), `model = "external-edit"`) || strings.Contains(string(now), "anchor_") {
		t.Fatal("external recovery baseline lost", err)
	}
}

func TestRealCoreCodexRebuildBridge(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".codex")
	os.Mkdir(root, 0700)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", root)
	path := filepath.Join(root, "config.toml")
	os.WriteFile(path, []byte("model = [broken"), 0600)
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	st, err := state.Open(s.options.Dir, false)
	if err != nil {
		t.Fatal(err)
	}
	secret := "synthetic"
	if _, err := st.PutProvider(state.Provider{ID: "p", BaseURL: "http://127.0.0.1:1", Protocol: "responses", Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, 0, &secret); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutModel(state.Model{ID: "public", Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture"}}}, 0); err != nil {
		t.Fatal(err)
	}
	st.Close()
	r := b.AgentWorkspace()
	if r.Problem != nil || r.Data == nil || r.Data.Targets[0].Status != "config_unsupported" || len(r.Data.Models[0].AvailableTargets) == 0 {
		t.Fatal("rebuild unavailable", r)
	}
	in := EnrollAgent{TargetID: r.Data.Targets[0].ID, Model: "public", ModelScope: "all-mapped", Approved: true, Rebuild: true}
	r = b.EnrollAgent(in)
	if r.Problem != nil || r.Data.Bindings[0].Status != "applied" {
		t.Fatal("rebuild failed", r)
	}
	in.ExpectedRevision = 0
	if r := b.EnrollAgent(in); r.Problem == nil || r.Problem.Code != "conflict" {
		t.Fatal("stale rebuild accepted", r)
	}
	now, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(now), `model = "public"`) {
		t.Fatal("configuration not replaced", err)
	}
}
