package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestAgentTokenAndIndependentRecovery(t *testing.T) {
	st, e := state.Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	if _, e = st.Initialize("127.0.0.1:17861", "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	_ = os.Chmod(root, 0700)
	exe, _ := os.Executable()
	b, e := st.AddBinding(state.Binding{ID: "local", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: root, Model: "public", Executable: exe, Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	engine := managed.NewEngine(st)
	p, e := (managed.Codex{}).Plan(b, managed.GatewayBinding{BaseURL: "http://127.0.0.1:17860/v1", Model: "public", DataDir: st.Dir, Ready: true, Tools: true, Streaming: true})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = engine.Apply(p); e != nil {
		t.Fatal(e)
	}
	var out, diagnostics bytes.Buffer
	a := App{In: strings.NewReader(""), Out: &out, Err: &diagnostics}
	ctx := context.Background()
	token, _ := st.BindingToken(b.ID)
	if e = a.Run(ctx, []string{"--data-dir", st.Dir, "agent", "token", b.ID}); e != nil || strings.TrimSpace(out.String()) != token {
		t.Fatal(out.String(), e)
	}
	out.Reset()
	if e = a.Run(ctx, []string{"--data-dir", st.Dir, "--context", "x", "agent", "token", b.ID}); e == nil {
		t.Fatal("context token allowed")
	}
	lock, e := platform.AcquireLock(filepath.Join(st.Dir, "instance.lock"))
	if e != nil {
		t.Fatal(e)
	}
	if e = a.Run(ctx, []string{"--data-dir", st.Dir, "recover"}); e == nil {
		t.Fatal("recovery raced core lock")
	}
	lock.Close()
	if e = a.Run(ctx, []string{"--data-dir", st.Dir, "recover", "--id", b.ID}); e != nil || !strings.Contains(out.String(), `"status":"restored"`) {
		t.Fatal(out.String(), e)
	}
	if strings.Contains(out.String(), token) {
		t.Fatal("recovery leaked token")
	}
	if e = a.Run(ctx, []string{"--data-dir", st.Dir, "recover", "--id", "missing"}); !errors.Is(e, state.ErrNotFound) {
		t.Fatal(e)
	}
}
func TestAgentHelpMatchesLifecycle(t *testing.T) {
	for _, s := range []string{"agent discover", "agent enroll", "--template", "agent sync", "agent add", "agent apply", "agent token", "recover [--id ID]", "--fingerprint", "--codex-home", "--approve", "--model-scope", "agent set-models", "--codex-catalog", "--codex-version"} {
		if !strings.Contains(help, s) {
			t.Fatal("missing help", s)
		}
	}
}

func TestCLICatalogImportAndModelScopes(t *testing.T) {
	st, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := core.New(st, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	if _, err = st.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	run := func(success bool, args ...string) string {
		t.Helper()
		var out, diagnostics bytes.Buffer
		app := App{In: strings.NewReader(""), Out: &out, Err: &diagnostics}
		err := app.Run(context.Background(), append([]string{"--data-dir", st.Dir}, args...))
		if success && err != nil {
			t.Fatal(args, err, diagnostics.String())
		}
		if !success && err == nil {
			t.Fatal("invalid command accepted", args)
		}
		return out.String()
	}
	run(true, "context", "add", "local", "--local")
	run(true, "context", "use", "local")
	run(true, "provider", "add", "--id", "p", "--base-url", "http://127.0.0.1:1234/v1")
	catalog, err := filepath.Abs("../state/testdata/codex-models.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{
		{"--codex-catalog", catalog},
		{"--codex-catalog", catalog, "--codex-model", "missing", "--codex-version", "0.159.3"},
		{"--codex-catalog", catalog, "--codex-model", "fixture-source", "--codex-version", "latest"},
	} {
		run(false, append([]string{"model", "add", "--id", "invalid", "--candidate", "p:actual"}, extra...)...)
	}
	for _, id := range []string{"default", "alternative"} {
		output := run(true, "model", "add", "--id", id, "--candidate", "p:actual", "--tools", "--streaming",
			"--codex-catalog", catalog, "--codex-model", "fixture-source", "--codex-version", "0.159.3")
		if !strings.Contains(output, `"codex"`) || !strings.Contains(output, `"version":"0.159.3"`) {
			t.Fatal(output)
		}
	}
	exe, _ := os.Executable()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	check := state.Binding{ID: "local", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: root, Executable: exe, Model: "default", ModelScope: "all-compatible", CodexVersion: "0.159.3", Enabled: true}
	if err := managed.ValidateTarget(&check); err != nil {
		t.Fatal("target", err)
	}
	if models, err := st.BindingModels(check); err != nil || len(models) != 2 {
		t.Fatal("eligible", models, err)
	}
	args := []string{"agent", "add", "--id", "local", "--codex-home", root, "--model", "default", "--anchor-executable", exe, "--approve"}
	run(false, append(args, "--model-scope=invalid")...)
	output := run(true, args...)
	if !strings.Contains(output, `"model_scope":"all-compatible"`) {
		t.Fatal(output)
	}
	token, _ := st.BindingToken("local")
	run(false, "agent", "set-models", "local", "--revision", "1", "--model", "alternative")
	output = run(true, "agent", "set-models", "local", "--revision", "1", "--model", "alternative", "--model-scope", "single")
	if !strings.Contains(output, `"apply_required":true`) {
		t.Fatal(output)
	}
	if now, _ := st.BindingToken("local"); now != token {
		t.Fatal("token changed")
	}
	// put is full replacement, including removal of omitted catalog metadata.
	run(true, "model", "put", "--id", "default", "--revision", "1", "--candidate", "p:actual", "--tools", "--streaming")
	m, err := st.Model("default")
	if err != nil || m.Codex != nil {
		t.Fatal(m, err)
	}
}

func TestCLIDiscoverEnrollSync(t *testing.T) {
	st, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := core.New(st, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	if _, err = st.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	root := filepath.Join(home, ".codex")
	os.Mkdir(root, 0700)
	exe, _ := os.Executable()
	srv.AgentEnvironment = func() (managed.Environment, error) {
		return managed.Environment{Home: home, CodexHome: root, Executable: exe}, nil
	}
	raw, err := os.ReadFile("../state/testdata/codex-models.json")
	if err != nil {
		t.Fatal(err)
	}
	var cache map[string]any
	json.Unmarshal(raw, &cache)
	cache["client_version"] = "0.159.3"
	raw, _ = json.Marshal(cache)
	os.WriteFile(filepath.Join(root, "models_cache.json"), raw, 0600)
	run := func(success bool, args ...string) string {
		t.Helper()
		var out, diag bytes.Buffer
		err := (App{In: strings.NewReader(""), Out: &out, Err: &diag}).Run(context.Background(), append([]string{"--data-dir", st.Dir}, args...))
		if success && err != nil {
			t.Fatal(args, err, diag.String())
		}
		if !success && err == nil {
			t.Fatal("invalid command accepted", args)
		}
		return out.String()
	}
	run(true, "context", "add", "local", "--local")
	run(true, "context", "use", "local")
	run(true, "provider", "add", "--id", "p", "--base-url", "http://127.0.0.1:1/v1")
	run(true, "model", "add", "--id", "public", "--candidate", "p:fixture-source", "--tools", "--streaming")
	var workspace managed.AgentWorkspace
	output := run(true, "agent", "discover")
	if json.Unmarshal([]byte(output), &workspace) != nil || len(workspace.Targets) != 1 {
		t.Fatal(output)
	}
	id := workspace.Targets[0].ID
	if len(workspace.Models) != 1 || len(workspace.Models[0].AvailableTargets) != 1 || workspace.Models[0].AvailableTargets[0] != id {
		t.Fatal("missing automatic model availability", output)
	}
	run(false, "agent", "enroll", "--target", id, "--model", "public")
	run(false, "agent", "enroll", "--target", id, "--model", "public", "--approve", "--template", "public:missing")
	output = run(true, "agent", "enroll", "--target", id, "--model", "public", "--approve")
	if json.Unmarshal([]byte(output), &workspace) != nil || workspace.Bindings[0].Status != "pending" {
		t.Fatal(output)
	}
	if listed := run(true, "agent", "list"); strings.Contains(listed, "base_instructions") || strings.Contains(listed, "codex_catalog") {
		t.Fatal("full catalog exposed in binding list")
	}
	srv.Agents.Start()
	run(false, "agent", "sync", id, "--revision", "0")
	run(true, "agent", "sync", id, "--revision", "1")
	run(false, "agent", "enroll", "--target", id, "--model", "public", "--approve")
	run(true, "agent", "disable", id, "--revision", "1")
	if _, err = os.Stat(filepath.Join(root, "anchor_"+id+".models.json")); !os.IsNotExist(err) {
		t.Fatal("catalog not restored", err)
	}
}
