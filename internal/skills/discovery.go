package skills

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/candyflosssss/Anchor/internal/managed"
)

// Directory evidence is not an assertion that an executable is installed.
// Discovery has no dependency on model-gateway enrollment.
func scopes(v manifest, env managed.Environment) []SkillScope {
	out, _ := scopesContext(context.Background(), v, env)
	return out
}

func scopesContext(ctx context.Context, v manifest, env managed.Environment) ([]SkillScope, []string) {
	out := []SkillScope{}
	seen := map[string]bool{}
	add := func(agent, kind, root, project, origin string) {
		root = filepath.Clean(root)
		if !absolutePath(root) || seen[root] {
			return
		}
		seen[root] = true
		out = append(out, SkillScope{key("scope-", root), agent, kind, root, project, origin})
	}
	if !filepath.IsAbs(env.Home) {
		return out, nil
	}
	add("shared", "global", filepath.Join(env.Home, ".agents", "skills"), "", "home")
	codex := env.CodexHome
	if codex == "" {
		codex = filepath.Join(env.Home, ".codex")
	}
	claude := env.ClaudeHome
	if claude == "" {
		claude = filepath.Join(env.Home, ".claude")
	}
	// Both default and configured instances are inventoried when distinct.
	for _, root := range []string{codex, filepath.Join(env.Home, ".codex")} {
		if sourceDir(root) == nil {
			add("codex", "global", filepath.Join(root, "skills"), "", "config-directory")
			add("codex", "system", filepath.Join(root, "skills", ".system"), "", "config-directory")
		}
	}
	for _, root := range []string{claude, filepath.Join(env.Home, ".claude")} {
		if sourceDir(root) == nil {
			add("claude", "global", filepath.Join(root, "skills"), "", "config-directory")
		}
	}
	for _, item := range []struct{ agent, dir string }{{"gemini", ".gemini"}, {"opencode", filepath.Join(".config", "opencode")}} {
		root := filepath.Join(env.Home, item.dir)
		if sourceDir(root) == nil {
			add(item.agent, "global", filepath.Join(root, "skills"), "", "config-directory")
		}
	}
	projects, warnings := discoverProjects(ctx, v, env)
	paths := []string{}
	for p := range projects {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if ctx.Err() != nil {
			break
		}
		if sourceDir(p) != nil {
			continue
		}
		for _, item := range projectSkillDirs {
			add(item.agent, "project", filepath.Join(p, item.dir, "skills"), p, projects[p])
		}
	}
	return out, warnings
}
func codexProjects(root string) []string {
	if sourceDir(root) != nil {
		return nil
	}
	path := filepath.Join(root, ".codex-global-state.json")
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() > 1<<20 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil || len(b) > 1<<20 {
		return nil
	}
	var index struct {
		Roots []string `json:"electron-saved-workspace-roots"`
		Order []string `json:"electron-workspace-root-order"`
	}
	if json.Unmarshal(b, &index) != nil {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	for _, p := range append(index.Roots, index.Order...) {
		if absolutePath(p) && p != filepath.VolumeName(p)+string(filepath.Separator) && !seen[p] {
			out = append(out, p)
			seen[p] = true
		}
	}
	return out
}
func (m *Manager) workspace(v manifest, env managed.Environment) SkillWorkspace {
	return m.workspaceContext(context.Background(), v, env)
}

func (m *Manager) workspaceContext(ctx context.Context, v manifest, env managed.Environment) SkillWorkspace {
	sc, warnings := scopesContext(ctx, v, env)
	ws := SkillWorkspace{Revision: v.Revision, Library: v.Library, Scopes: sc, Sources: []SkillSource{}, Mounts: []SkillMount{}, Warnings: append([]string{}, warnings...)}
	if v.Pending != nil || v.Cleanup != nil {
		ws.Warnings = append(ws.Warnings, "pending_transaction")
	}
	associations := map[string]association{}
	for _, a := range v.Sources {
		associations[a.Path] = a
	}
	mountPaths := map[string]SkillMount{}
	for _, mt := range v.Mounts {
		mountPaths[mt.Path] = mt
	}

	emitted := map[string]bool{}
	visited := map[string]bool{}
	for _, sc := range ws.Scopes {
		if absent(sc.Root) {
			continue
		}
		if sourceDir(sc.Root) != nil {
			ws.Warnings = append(ws.Warnings, "unsafe_scope:"+sc.ID)
			continue
		}
		f, err := os.Open(sc.Root)
		if err != nil {
			ws.Warnings = append(ws.Warnings, "unreadable_scope:"+sc.ID)
			continue
		}
		entries, e := f.ReadDir(maxInventory + 1)
		f.Close()
		if len(entries) > maxInventory {
			ws.Warnings = append(ws.Warnings, "scope_limit:"+sc.ID)
			entries = entries[:maxInventory]
		}
		if e != nil && !errors.Is(e, io.EOF) {
			ws.Warnings = append(ws.Warnings, "unreadable_scope:"+sc.ID)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if ctx.Err() != nil {
				return ws
			}
			if strings.HasPrefix(entry.Name(), ".anchor-skills-") || entry.Name() == ".system" || strings.HasPrefix(entry.Name(), ".") && !entry.IsDir() {
				continue
			}
			if len(ws.Sources) >= maxInventory {
				ws.Warnings = append(ws.Warnings, "source_limit")
				break
			}
			path := filepath.Join(sc.Root, entry.Name())
			if !absolutePath(path) {
				ws.Warnings = append(ws.Warnings, "path_limit:"+sc.ID)
				continue
			}
			if visited[path] {
				continue
			}
			visited[path] = true
			if _, ok := mountPaths[path]; ok {
				continue
			}
			src := SkillSource{ID: key("source-", path), ScopeID: sc.ID, Path: path, Name: entry.Name(), Status: "local"}
			if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			if _, err := os.Lstat(filepath.Join(path, "SKILL.md")); err != nil {
				continue
			}
			if a, ok := associations[path]; ok {
				src.SkillID = a.SkillID
				src.Baseline = a.Version
			}
			t, err := readSourceContext(ctx, path)
			if err != nil {
				src.Status = "unsafe"
				if errors.Is(err, ErrLimit) {
					src.Status = "limit"
				}
			} else {
				src.Hash = t.Hash
				if src.SkillID != "" {
					s := findSkill(&v, src.SkillID)
					base := findVersion(s, src.Baseline)
					if base == nil {
						src.Status = "unavailable"
					} else {
						latest := s.Versions[len(s.Versions)-1]
						sourceChanged := t.Hash != base.Hash
						libraryChanged := latest.Hash != base.Hash
						switch {
						case t.Hash == latest.Hash:
							src.Status = "in_sync"
						case sourceChanged && libraryChanged:
							src.Status = "diverged"
						case sourceChanged:
							src.Status = "source_changed"
						case libraryChanged:
							src.Status = "library_changed"
						default:
							src.Status = "imported"
						}
					}
				}
			}
			for _, excluded := range v.Excluded {
				if excluded == path && src.SkillID == "" {
					src.Status = "excluded"
				}
			}
			ws.Sources = append(ws.Sources, src)
			emitted[src.Path] = true
		}
	}
	for _, a := range v.Sources {
		if _, mounted := mountPaths[a.Path]; !emitted[a.Path] && !mounted {
			ws.Sources = append(ws.Sources, SkillSource{ID: key("source-", a.Path), ScopeID: key("scope-", filepath.Dir(a.Path)), Path: a.Path, Name: filepath.Base(a.Path), SkillID: a.SkillID, Baseline: a.Version, Status: "missing"})
		}
	}
	for _, mt := range v.Mounts {
		mt.Status = "linked"
		if !linkMatches(mt.Path, mt.Target) {
			if absent(mt.Path) {
				mt.Status = "missing"
			} else {
				mt.Status = "conflict"
			}
		}
		s := findSkill(&v, mt.SkillID)
		if findVersion(s, mt.Version) == nil || absent(mt.Target) {
			if mt.Status == "linked" {
				mt.Status = "broken"
			}
		}
		ws.Mounts = append(ws.Mounts, mt)
	}
	return ws
}
