package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/skills"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestSkillCLIUsesAuthenticatedCore(t *testing.T) {
	st, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := core.New(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer srv.Close()
	home := t.TempDir()
	_ = os.Chmod(home, 0700)
	srv.AgentEnvironment = func() (managed.Environment, error) { return managed.Environment{Home: home}, nil }
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	_, err = st.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860")
	if err != nil {
		t.Fatal(err)
	}
	var out, diag bytes.Buffer
	a := App{In: strings.NewReader("# Example\n"), Out: &out, Err: &diag}
	run := func(args ...string) error {
		out.Reset()
		return a.Run(context.Background(), append([]string{"--data-dir", st.Dir}, args...))
	}
	for _, args := range [][]string{{"context", "add", "local", "--local"}, {"context", "use", "local"}, {"skill", "scan"}} {
		if err = run(args...); err != nil {
			t.Fatal(err, diag.String())
		}
	}
	if err = run("skill", "create", "example", "--content-stdin", "--revision", "1"); err == nil {
		t.Fatal("approval not required")
	}
	if err = run("skill", "create", "example", "--content-stdin", "--revision", "1", "--approve"); err != nil {
		t.Fatal(err, diag.String())
	}
	var ws skills.SkillWorkspace
	if json.Unmarshal(out.Bytes(), &ws) != nil || len(ws.Library) != 1 {
		t.Fatal(out.String())
	}
	if err = run("skill", "inspect", ""); err == nil {
		t.Fatal("empty identity accepted")
	}
	if err = run("skill", "create", "bad", "--file", "other.md", "--content-stdin", "--revision", "2", "--approve"); err == nil {
		t.Fatal("create accepted a non-SKILL.md path")
	}
	id := ws.Library[0].ID
	if err = run("skill", "inspect", id, "--version", "1", "--file", "SKILL.md"); err != nil {
		t.Fatal(err)
	}
	var d skills.SkillDetail
	_ = json.Unmarshal(out.Bytes(), &d)
	if d.Content != "# Example\n" {
		t.Fatal(d)
	}
	if err = run("skill", "save", id, "--version", "1", "--content-stdin", "--revision", "1", "--approve"); err == nil {
		t.Fatal("stale write accepted")
	}
	if err = run("skill", "link", id, "--version", "1", "--scope", ws.Scopes[0].ID, "--revision", "2"); err == nil {
		t.Fatal("link without approval")
	}
	if runtime.GOOS != "windows" {
		source := filepath.Join(home, ".agents", "skills", "external")
		if err = os.MkdirAll(source, 0775); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("external"), 0664); err != nil {
			t.Fatal(err)
		}
		if err = run("skill", "scan"); err != nil {
			t.Fatal(err)
		}
		if json.Unmarshal(out.Bytes(), &ws) != nil || len(ws.Library) != 2 || len(ws.Sources) != 1 {
			t.Fatal(out.String())
		}
		src := ws.Sources[0]
		revision := strconv.FormatInt(ws.Revision, 10)
		if err = run("skill", "cleanup", src.ID, "--revision", revision, "--approve"); err == nil {
			t.Fatal("cleanup without hash")
		}
		if err = run("skill", "cleanup", src.ID, "--hash", src.Hash, "--revision", revision); err == nil {
			t.Fatal("cleanup without approval")
		}
		if err = run("skill", "cleanup", src.ID, "--hash", src.Hash, "--revision", revision, "--approve"); err != nil {
			t.Fatal(err)
		}
		if json.Unmarshal(out.Bytes(), &ws) != nil || len(ws.Mounts) != 1 {
			t.Fatal(out.String())
		}
		if link, err := os.Readlink(source); err != nil || link != ws.Mounts[0].Target {
			t.Fatal(link, err)
		}
	}
	count := len(ws.Library)
	project := filepath.Join(home, "Projects", "automatically-found")
	source := filepath.Join(project, ".codex", "skills", "project-skill")
	if err = os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("project skill"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = run("skill", "scan"); err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(out.Bytes(), &ws) != nil || len(ws.Library) != count+1 {
		t.Fatal("CLI required manual project registration", out.String())
	}
	if st, err := os.Lstat(source); err != nil || !st.IsDir() {
		t.Fatal("CLI scan replaced a project source", err)
	}
}

func TestSkillContentCannotShareSSHPasswordStdin(t *testing.T) {
	input := strings.NewReader("# Do not consume\n")
	var out, diag bytes.Buffer
	a := App{In: input, Out: &out, Err: &diag}
	err := a.Run(context.Background(), []string{"--ssh-password-stdin", "skill", "create", "example", "--content-stdin", "--revision", "1", "--approve"})
	if err == nil || !strings.Contains(err.Error(), "cannot share stdin") {
		t.Fatalf("stdin collision not rejected: %v", err)
	}
	if input.Len() != len("# Do not consume\n") {
		t.Fatal("input consumed before rejecting conflicting stdin modes")
	}
}

func TestSkillHelpDescribesAutomaticProjectDiscovery(t *testing.T) {
	for _, hint := range []string{"Automatically find direct projects under ~/Projects", "no registration needed", "Optionally add a project outside automatic discovery"} {
		if !strings.Contains(help, hint) {
			t.Fatal("missing automatic project discovery help", hint)
		}
	}
}
