package managed

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/candyflosssss/Anchor/internal/platform"
)

// CodexLogProject resolves only structured project metadata for one exact thread.
// It never scans rollouts, reads conversation text or modifies Codex state. Empty
// means unavailable, not a guessed project. The caller must authenticate a Codex binding.
func CodexLogProject(ctx context.Context, root, thread string) string {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	raw, _, err := readConfig(root, filepath.Join(root, "config.toml"))
	if err != nil {
		return ""
	}
	config, err := document(raw)
	if err != nil {
		return ""
	}
	if configured, ok := config["sqlite_home"]; ok {
		dir, ok := configured.(string)
		if !ok || dir == "" {
			return ""
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, dir)
		}
		root = filepath.Clean(dir)
	}
	// Bound directory enumeration and use the newest version, never an old DB
	// which may retain stale thread ownership after an upgrade.
	dir, err := os.Open(root)
	if err != nil {
		return ""
	}
	entries, _ := dir.ReadDir(257)
	dir.Close()
	if len(entries) > 256 {
		return ""
	}
	version, path := -1, ""
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && strings.HasPrefix(name, "state_") && strings.HasSuffix(name, ".sqlite") {
			n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "state_"), ".sqlite"))
			if err == nil && n >= 0 && n > version {
				version, path = n, filepath.Join(root, name)
			}
		}
	}
	if path == "" || platform.CheckManagedPath(root, path) != nil {
		return ""
	}
	// Read-only with a short lock wait; never create a missing database or retain
	// a connection across requests. query_only also guards accidental future writes.
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	q := u.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", "busy_timeout(20)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return ""
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	// Desktop project collections take precedence when available. Older Codex
	// schemas and threads outside a collection fall back to their recorded cwd.
	var id, name string
	err = db.QueryRowContext(ctx, `SELECT substr(p.id,1,129),substr(p.name,1,129) FROM threads t JOIN projects p ON p.id=t.project_id WHERE t.id=?`, thread).Scan(&id, &name)
	if err == nil && id != "" && len(id) <= 128 && name != "" && len(name) <= 128 && !strings.ContainsAny(name, `/\`) {
		return codexProjectLabel(name, "project:"+id)
	}
	var cwd string
	if db.QueryRowContext(ctx, `SELECT substr(cwd,1,4097) FROM threads WHERE id=?`, thread).Scan(&cwd) != nil || len(cwd) > 4096 || !filepath.IsAbs(cwd) {
		return ""
	}
	cwd = filepath.Clean(cwd)
	if cwd == filepath.VolumeName(cwd)+string(filepath.Separator) {
		return ""
	}
	return codexProjectLabel(filepath.Base(cwd), "cwd:"+cwd)
}

// A readable prefix makes existing prefix suggestions useful. A path/ID digest
// distinguishes same-named workspaces without storing full paths in Anchor logs.
func codexProjectLabel(name, identity string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, "-_.")
	for len(name) > 96 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	if name == "" {
		name = "project"
	}
	hash := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("%s@%x", name, hash[:6])
}
