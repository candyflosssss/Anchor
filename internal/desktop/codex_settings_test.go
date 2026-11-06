package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/state"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCodexSettingsBridgeAndCLI(t *testing.T) {
	s := testSession(t)
	defer s.Close()
	b := NewBridge(s)
	requireReady(t, b)
	initial := b.CodexSettings()
	if initial.Problem != nil || initial.Data == nil || initial.Data.Settings.Retries != 5 || initial.Data.Settings.OverrideModelCatalog {
		t.Fatal(initial)
	}
	n := 0
	in := SaveCodexSettings{Retries: &n, ExpectedRevision: initial.Data.Settings.Revision}
	out := b.SaveCodexSettings(in)
	if out.Problem != nil || out.Data == nil || out.Data.Settings.Retries != 0 || out.Data.Settings.Revision != 2 {
		t.Fatal(out)
	}
	if stale := b.SaveCodexSettings(in); stale.Problem == nil || stale.Problem.Code != "conflict" {
		t.Fatal(stale)
	}
	n = 11
	if invalid := b.SaveCodexSettings(in); invalid.Problem == nil || invalid.Problem.Code != "invalid" {
		t.Fatal(invalid)
	}
	run := func(wantErr bool, args ...string) string {
		t.Helper()
		var output bytes.Buffer
		err := (cli.App{Out: &output, Err: io.Discard, In: strings.NewReader("")}).Run(context.Background(), append([]string{"--data-dir", s.options.Dir}, args...))
		if (err != nil) != wantErr {
			t.Fatal(args, err)
		}
		return output.String()
	}
	run(false, "context", "add", "local-test", "--local")
	run(false, "context", "use", "local-test")
	if got := run(false, "agent", "codex-settings"); !strings.Contains(got, `"retries":0`) {
		t.Fatal(got)
	}
	if got := run(false, "agent", "codex-settings", "--retries", "3", "--revision", "2"); !strings.Contains(got, `"retries":3`) || !strings.Contains(got, `"revision":3`) {
		t.Fatal(got)
	}
	run(true, "agent", "codex-settings", "--retries", "4", "--revision", "2")
	run(true, "agent", "codex-settings", "--retries", "11", "--revision", "3")
	run(true, "agent", "codex-settings", "--retries", "3")
	if got := run(false, "agent", "codex-settings", "--auto-compact=false", "--compact-percent", "80", "--revision", "3"); !strings.Contains(got, `"auto_compact":false`) || !strings.Contains(got, `"compact_percent":80`) || !strings.Contains(got, `"retries":3`) {
		t.Fatal(got)
	}
	run(true, "agent", "codex-settings", "--compact-percent", "100", "--revision", "4")
	run(true, "agent", "codex-settings", "--auto-compact=false")
	run(true, "agent", "codex-settings", "--revision", "4")
	if got := run(false, "agent", "codex-settings", "--override-model-catalog=true", "--revision", "4"); !strings.Contains(got, `"override_model_catalog":true`) {
		t.Fatal(got)
	}
	if read := b.CodexSettings(); read.Problem != nil || !read.Data.Settings.OverrideModelCatalog {
		t.Fatal(read)
	}
	off := false
	if saved := b.SaveCodexSettings(SaveCodexSettings{OverrideModelCatalog: &off, ExpectedRevision: 5}); saved.Problem != nil || saved.Data.Settings.OverrideModelCatalog {
		t.Fatal(saved)
	}

}
func TestSharedCodexSettingsFixture(t *testing.T) {
	for _, path := range []string{"../../desktop/contract/codex-settings.json", "../../desktop/contract/codex-settings-incomplete.json"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var out CodexSettingsState
		if err = json.Unmarshal(raw, &out); err != nil || out.Settings.Retries != 5 || out.Settings.Revision != 1 || out.NeedsSync == nil || out.Compaction == nil {
			t.Fatal(out, err)
		}
		encoded, _ := json.Marshal(out)
		var a, b any
		json.Unmarshal(raw, &a)
		json.Unmarshal(encoded, &b)
		if !reflect.DeepEqual(a, b) {
			t.Fatal("fixture drift")
		}
	}
}

func TestCodexCompactionMissingContextAcrossBridgeAndCLI(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".codex")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("model = \"original\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", root)
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
	workspace := b.AgentWorkspace()
	if workspace.Problem != nil || workspace.Data == nil || len(workspace.Data.Targets) != 1 {
		t.Fatal(workspace)
	}
	on := true
	if opted := b.SaveCodexSettings(SaveCodexSettings{OverrideModelCatalog: &on, ExpectedRevision: 1}); opted.Problem != nil {
		t.Fatal(opted)
	}
	applied := b.EnrollAgent(EnrollAgent{TargetID: workspace.Data.Targets[0].ID, Model: "public", ModelScope: "all-mapped", Approved: true})
	if applied.Problem != nil || applied.Data == nil || applied.Data.Bindings[0].Status != "applied" {
		t.Fatal(applied)
	}
	read := b.CodexSettings()
	if read.Problem != nil || read.Data == nil || len(read.Data.Compaction) != 1 || read.Data.Compaction[0].Status != "missing_context" || len(read.Data.NeedsSync) != 0 {
		t.Fatal(read)
	}
	var stdout bytes.Buffer
	run := func(args ...string) error {
		stdout.Reset()
		return (cli.App{Out: &stdout, Err: io.Discard, In: strings.NewReader("")}).Run(context.Background(), append([]string{"--data-dir", s.options.Dir}, args...))
	}
	if err = run("context", "add", "local-test", "--local"); err != nil {
		t.Fatal(err)
	}
	if err = run("context", "use", "local-test"); err != nil {
		t.Fatal(err)
	}
	err = run("agent", "codex-settings", "--compact-percent", "80", "--revision", "2")
	if err == nil || !strings.Contains(err.Error(), "settings saved; automatic compaction is incomplete") || !strings.Contains(stdout.String(), `"status":"missing_context"`) {
		t.Fatal(stdout.String(), err)
	}
	saved := b.CodexSettings()
	if saved.Problem != nil || saved.Data.Settings.Revision != 3 || saved.Data.Settings.CompactPercent != 80 {
		t.Fatal(saved)
	}
	// A successful model save synchronizes the same directory and clears the
	// missing-context report without touching real Agent files or an upstream.
	window := int64(272000)
	output := b.SaveModelSettings(SaveModelSettings{ModelID: "public", ExpectedRevision: 1, ReferenceAction: "keep", Configuration: ModelConfiguration{Limits: CatalogLimits{Context: &window}}})
	if output.Problem != nil {
		t.Fatal(output)
	}
	read = b.CodexSettings()
	if read.Problem != nil || read.Data.Compaction[0].Status != "written" || *read.Data.Compaction[0].TokenLimit != 217600 {
		t.Fatal(read)
	}
	if err = run("agent", "codex-settings", "--compact-percent", "90", "--revision", "3"); err != nil || !strings.Contains(stdout.String(), `"token_limit":244800`) {
		t.Fatal(stdout.String(), err)
	}
}
