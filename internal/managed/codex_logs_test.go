package managed

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

const codexFixtureThread = "01a14453-be7d-7881-afc3-dc082aa5c529"

func codexLogFixture(t *testing.T, root string, collections bool) string {
	t.Helper()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "state_5.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE threads(id TEXT PRIMARY KEY,cwd TEXT,title TEXT,project_id TEXT);
INSERT INTO threads VALUES('` + codexFixtureThread + `','` + filepath.ToSlash(filepath.Join(root, "private", "Anchor")) + `','DO_NOT_READ_MESSAGES','collection-1');`)
	if err != nil {
		t.Fatal(err)
	}
	if collections {
		if _, err := db.Exec(`CREATE TABLE projects(id TEXT PRIMARY KEY,name TEXT); INSERT INTO projects VALUES('collection-1','我的 Anchor 项目');`); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCodexLogProject(t *testing.T) {
	for _, collections := range []bool{false, true} {
		t.Run(map[bool]string{false: "cwd", true: "collection"}[collections], func(t *testing.T) {
			root := t.TempDir()
			path := codexLogFixture(t, root, collections)
			before, _ := os.ReadFile(path)
			project := CodexLogProject(t.Context(), root, codexFixtureThread)
			prefix := "Anchor@"
			if collections {
				prefix = "我的-Anchor-项目@"
			}
			if !strings.HasPrefix(project, prefix) || len(project) > 128 || strings.Contains(project, root) {
				t.Fatalf("unexpected label %q", project)
			}
			if again := CodexLogProject(t.Context(), root, codexFixtureThread); again != project {
				t.Fatal("unstable identity")
			}
			if got := CodexLogProject(t.Context(), root, "missing"); got != "" {
				t.Fatal("guessed missing thread")
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("Codex database modified")
			}
		})
	}
	if a, b := codexProjectLabel("Anchor", "cwd:/one/Anchor"), codexProjectLabel("Anchor", "cwd:/two/Anchor"); a == b {
		t.Fatal("same-named workspaces conflated")
	}
	label := codexProjectLabel(strings.Repeat("中文", 100)+"\n/private", "id")
	if !utf8.ValidString(label) || len(label) > 128 || strings.ContainsAny(label, "/\n") {
		t.Fatal("unsafe project label")
	}
}

func TestCodexLogProjectCustomHomeAndUnavailable(t *testing.T) {
	root, dbroot := t.TempDir(), t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	codexLogFixture(t, dbroot, false)
	// Relative sqlite_home is resolved against the registered configuration root.
	rel, _ := filepath.Rel(root, dbroot)
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("sqlite_home = '"+rel+"'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := CodexLogProject(t.Context(), root, codexFixtureThread); !strings.HasPrefix(got, "Anchor@") {
		t.Fatal(got)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if CodexLogProject(ctx, root, codexFixtureThread) != "" {
		t.Fatal("ignored cancellation")
	}
	if err := os.WriteFile(filepath.Join(dbroot, "state_6.sqlite"), []byte("unsupported database"), 0600); err != nil {
		t.Fatal(err)
	}
	if CodexLogProject(t.Context(), root, codexFixtureThread) != "" {
		t.Fatal("used stale database")
	}
	missing := t.TempDir()
	if CodexLogProject(t.Context(), missing, codexFixtureThread) != "" {
		t.Fatal("guessed missing database")
	}
	entries, _ := os.ReadDir(missing)
	if len(entries) != 0 {
		t.Fatal("created Codex state")
	}
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("malformed = ["), 0600); err != nil {
		t.Fatal(err)
	}
	if CodexLogProject(t.Context(), root, codexFixtureThread) != "" {
		t.Fatal("ignored invalid configuration")
	}
}

func TestCodexLogProjectRejectsLinkedDatabase(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	path := codexLogFixture(t, other, false)
	if err := os.Symlink(path, filepath.Join(root, "state_5.sqlite")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if CodexLogProject(t.Context(), root, codexFixtureThread) != "" {
		t.Fatal("followed linked database")
	}
}
