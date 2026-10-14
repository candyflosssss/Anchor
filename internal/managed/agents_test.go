package managed

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestMappedAgentLifecycle(t *testing.T) {
	for _, adapter := range []string{"codex", "claude", "dsh"} {
		t.Run(adapter, func(t *testing.T) {
			m, env, _ := workspaceManager(t)
			if _, err := m.Store.PutModel(state.Model{ID: "plain-alias", Candidates: []state.Candidate{{ProviderID: "p", Model: "upstream-without-metadata"}}}, 0); err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(env.Home, "."+adapter)
			if err := os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			var path string
			var original []byte
			switch adapter {
			case "codex":
				path = filepath.Join(root, "config.toml")
				original = []byte("model = \"original\"\nuser = \"keep\"\n")
			case "claude":
				path = filepath.Join(root, "settings.json")
				original = []byte(`{"model":"original","env":{"USER_SETTING":"keep","ANTHROPIC_AUTH_TOKEN":"old-private","ANTHROPIC_API_KEY":"old-key"},"hooks":{"Stop":[]},"permissions":{"allow":["Read"]}}`)
			case "dsh":
				path = filepath.Join(root, "profiles", "web", "cordis.patch.yml")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				original = []byte("# user comment\n- id: llm-pi-ai\n  config:\n    providers:\n      original:\n        api: openai-completions\n        baseURL: https://example.invalid/v1\n- id: agent-default-model\n  config:\n    provider: original\n    model: original-model\n    user: keep\n- id: unrelated\n  config:\n    expression: !!js '() => 1'\n")
			}
			if err := os.WriteFile(path, original, 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Start(); err != nil {
				t.Fatal(err)
			}
			defer m.Stop()
			targets, err := Discover(env)
			if err != nil {
				t.Fatal(err)
			}
			targetID := ""
			for _, target := range targets {
				if target.Agent == adapter && target.ConfigRoot == root && target.Status == "ready" {
					targetID = target.ID
				}
			}
			if targetID == "" {
				t.Fatal(targets)
			}
			b, err := m.Enroll(env, EnrollAgent{TargetID: targetID, Model: "plain-alias", ModelScope: "all-mapped", Approved: true})
			if err != nil {
				t.Fatal(err)
			}
			token, err := m.Store.BindingToken(b.ID)
			if err != nil {
				t.Fatal(err)
			}
			g, err := m.gateway(b)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = AdapterFor(adapter).Verify(b, g); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(raw, []byte("keep")) {
				t.Fatal("unrelated field lost")
			}
			if adapter != "codex" {
				if !bytes.Contains(raw, []byte(token)) {
					t.Fatal("scoped credential not written")
				}
				info, _ := os.Stat(path)
				if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
					t.Fatal("credential config not private", info.Mode())
				}
			}
			if adapter == "claude" {
				var cfg map[string]any
				if err := json.Unmarshal(raw, &cfg); err != nil {
					t.Fatal(err)
				}
				e := cfg["env"].(map[string]any)
				if e["ANTHROPIC_BASE_URL"] == g.BaseURL {
					t.Fatal("Claude SDK would duplicate /v1")
				}
				if e["ANTHROPIC_AUTH_TOKEN"] != token || e["ANTHROPIC_API_KEY"] != "" {
					t.Fatal("credential precedence")
				}
			}
			if adapter == "dsh" && !bytes.Contains(raw, []byte("!!js")) {
				t.Fatal("custom YAML tag lost")
			}
			workspace, err := m.Workspace(env)
			if err != nil {
				t.Fatal(err)
			}
			public, _ := json.Marshal(workspace)
			if bytes.Contains(public, []byte(token)) || bytes.Contains(public, []byte("old-private")) {
				t.Fatal("workspace leaked credential")
			}
			// All mapped aliases authorize dynamically, including models lacking flags or metadata.
			if ok, err := m.Store.BindingAllows(b, "public"); err != nil || !ok {
				t.Fatal(ok, err)
			}
			if ok, _ := m.Store.BindingAllows(b, "unknown"); ok {
				t.Fatal("unknown model authorized")
			}
			// Synchronization must rebuild client lists while keeping original recovery values.
			if _, err := m.Store.PutModel(state.Model{ID: "added-after-enroll", Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}, 0); err != nil {
				t.Fatal(err)
			}
			if r, err := m.Refresh(b.ID, b.Revision); err != nil || r.Status != "applied" {
				t.Fatal(r, err)
			}
			g, err = m.gateway(b)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := AdapterFor(adapter).Verify(b, g); err != nil {
				t.Fatal(err)
			}
			raw, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if adapter != "codex" && !bytes.Contains(raw, []byte("added-after-enroll")) {
				t.Fatal("new mapping not synchronized")
			}
			// An unrelated edit must survive conditional restore.
			switch adapter {
			case "codex":
				raw = append([]byte("new_user = \"preserve\"\n"), raw...)
			case "claude":
				var cfg map[string]any
				_ = json.Unmarshal(raw, &cfg)
				cfg["new_user"] = "preserve"
				raw, _ = json.Marshal(cfg)
			case "dsh":
				raw = append(raw, []byte("- id: new-user\n  config:\n    text: preserve\n")...)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			disabled, result, err := m.SetEnabled(b.ID, b.Revision, false)
			if err != nil || disabled.Enabled || result.Status != "restored" {
				t.Fatal(disabled, result, err)
			}
			restored, _ := os.ReadFile(path)
			if !bytes.Contains(restored, []byte("preserve")) || bytes.Contains(restored, []byte(token)) {
				t.Fatal("restore lost user edit or retained token")
			}
			if _, err := m.Store.BindingForToken(token); !errors.Is(err, state.ErrNotFound) {
				t.Fatal("disabled credential remains usable", err)
			}
			if adapter == "claude" && !bytes.Contains(restored, []byte("old-private")) {
				t.Fatal("original auth not restored")
			}
			if adapter == "dsh" && (!bytes.Contains(restored, []byte("original-model")) || !bytes.Contains(restored, []byte("!!js"))) {
				t.Fatal("DSH original not restored")
			}
		})
	}
}

func TestStructuredRecoveryConflictAndCreatedRows(t *testing.T) {
	for _, dsh := range []bool{false, true} {
		t.Run(map[bool]string{false: "claude", true: "dsh"}[dsh], func(t *testing.T) {
			m, env, _ := workspaceManager(t)
			root := filepath.Join(env.Home, "structured")
			path := filepath.Join(root, "settings.json")
			adapter := "claude"
			original := []byte(`{"env":{"USER":"keep"}}`)
			if dsh {
				adapter = "dsh"
				path = filepath.Join(root, "profiles", "web", "cordis.patch.yml")
				original = []byte("- id: untouched\n  config: {}\n")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			b, err := m.Add(state.Binding{ID: "structured", AdapterID: adapter, AdapterVersion: "1", ConfigRoot: root, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.Start(); err != nil {
				t.Fatal(err)
			}
			defer m.Stop()
			raw, _ := os.ReadFile(path)
			editor := StructuredEditor{DSH: dsh}
			field := []string{"model"}
			if dsh {
				field = []string{"agent-default-model", "config", "model"}
			}
			v, err := editor.Values(raw, [][]string{field})
			if err != nil {
				t.Fatal(err)
			}
			raw, err = editor.Patch(raw, []Edit{{Path: field, Before: v[0], After: Value{Exists: true, Data: "user-choice"}}})
			if err != nil {
				t.Fatal(err)
			}
			_ = os.WriteFile(path, raw, 0600)
			_, result, err := m.SetEnabled(b.ID, b.Revision, false)
			if err != nil || result.Status != "conflict" {
				t.Fatal(result, err)
			}
			raw, _ = os.ReadFile(path)
			if !bytes.Contains(raw, []byte("user-choice")) {
				t.Fatal("third-party change overwritten")
			}
			if dsh && (!bytes.Contains(raw, []byte("config: {}")) || bytes.Contains(raw, []byte("id: llm-pi-ai"))) {
				t.Fatal("untouched empty config deleted or created route retained", string(raw))
			}
		})
	}
}

func TestStructuredRejectAmbiguousDocuments(t *testing.T) {
	for _, raw := range []string{`{"model":"a","model":"b"}`, `{"env":null}`, `[]`} {
		if _, err := (StructuredEditor{}).Values([]byte(raw), [][]string{{"env", "ANTHROPIC_MODEL"}}); err == nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"- id: duplicate\n- id: duplicate\n", "- id: a\n  config: &shared {}\n- id: b\n  config: *shared\n"} {
		if _, err := (StructuredEditor{DSH: true}).parse([]byte(raw)); err == nil {
			t.Fatal(raw)
		}
	}
}

func TestStructuredReservedRouteAndInvalidEnvironment(t *testing.T) {
	m, env, _ := workspaceManager(t)
	root := filepath.Join(env.Home, "config")
	path := filepath.Join(root, "settings.json")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"env":null}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (StructuredAgent{}).Inspect(state.Binding{ConfigRoot: root}); err == nil {
		t.Fatal("invalid env accepted")
	}
	path = filepath.Join(root, "profiles", "web", "cordis.patch.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("- id: llm-pi-ai\n  config:\n    providers:\n      anchor-collision: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := m.Add(state.Binding{ID: "collision", AdapterID: "dsh", AdapterVersion: "1", ConfigRoot: root, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	m.ready = true
	if _, err := m.Plan(b.ID); err == nil {
		t.Fatal("reserved route overwritten")
	}
}
