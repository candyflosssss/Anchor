package skills

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/candyflosssss/Anchor/internal/managed"
)

// These are discovery/work bounds, not a per-Skill content budget. Explicit
// registrations keep their existing separate 64-project manifest limit.
const maxDiscoveredProjects = 1024
const maxProjectSearchEntries = 4096

var projectSkillDirs = []struct{ agent, dir string }{
	{"shared", ".agents"}, {"claude", ".claude"}, {"codex", ".codex"},
	{"gemini", ".gemini"}, {"opencode", ".opencode"},
}

// Common project containers are inspected one level only. Never descend into
// repositories, dependencies, hidden directories, chats or arbitrary home data.
var projectContainers = []string{
	"Projects", "projects", "Code", "code", "src", "repos", "Repositories",
	"Work", "work", "Workspace", "workspace", "workspaces",
	filepath.Join("Documents", "GitHub"), filepath.Join("source", "repos"),
}

func projectIdentity(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

func projectEvidence(path string) bool {
	// .git may be a regular worktree pointer, not just a directory.
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return true
	}
	for _, item := range projectSkillDirs {
		if st, err := os.Stat(filepath.Join(path, item.dir)); err == nil && st.IsDir() {
			return true
		}
	}
	return false
}

// This is the effective project registry, refreshed on every collection pass.
// Automatic discovery does not need to mutate the explicit-registration list,
// nor enroll an Agent, nor edit a project's files/configuration.
func discoverProjects(ctx context.Context, v manifest, env managed.Environment) (map[string]string, []string) {
	projects := map[string]string{}
	warnings := []string{}
	if !absolutePath(env.Home) || ctx.Err() != nil {
		return projects, warnings
	}
	seen := map[string]bool{}
	limited := false
	add := func(path, origin string) {
		if ctx.Err() != nil || !absolutePath(path) || path == filepath.VolumeName(path)+string(filepath.Separator) || sourceDir(path) != nil {
			return
		}
		identity := projectIdentity(path)
		if projects[path] != "" || seen[identity] && origin != "registered" && origin != "archived-source" {
			return
		}
		if len(projects) >= maxDiscoveredProjects {
			limited = true
			return
		}
		seen[identity] = true
		projects[path] = origin
	}
	// Keep explicit aliases usable by old source/mount records. Automatic
	// candidates deduplicate by real directory, and cannot displace these roots.
	for _, path := range v.Projects {
		add(path, "registered")
	}
	// Source associations also remember automatically found projects. Keep their
	// original spelling available for cleanup even if an index/alias later changes.
	remember := func(path string) {
		if ctx.Err() != nil {
			return
		}
		skillRoot := filepath.Dir(path)
		agentRoot := filepath.Dir(skillRoot)
		project := filepath.Dir(agentRoot)
		if filepath.Base(skillRoot) != "skills" || projectIdentity(project) == projectIdentity(env.Home) ||
			projectIdentity(agentRoot) == projectIdentity(env.CodexHome) || projectIdentity(agentRoot) == projectIdentity(env.ClaudeHome) {
			return
		}
		for _, item := range projectSkillDirs {
			if filepath.Base(agentRoot) == item.dir {
				add(project, "archived-source")
				return
			}
		}
	}
	for _, source := range v.Sources {
		remember(source.Path)
	}
	for _, mount := range v.Mounts {
		remember(mount.Path)
	}
	codex := env.CodexHome
	if codex == "" {
		codex = filepath.Join(env.Home, ".codex")
	}
	indexes := map[string]bool{}
	for _, root := range []string{codex, filepath.Join(env.Home, ".codex")} {
		if ctx.Err() != nil {
			return projects, warnings
		}
		identity := projectIdentity(root)
		if indexes[identity] {
			continue
		}
		indexes[identity] = true
		for _, path := range codexProjects(root) {
			add(path, "codex-workspace-index")
		}
	}
	containers := map[string]bool{}
	remaining := maxProjectSearchEntries
	searchLimited := false
	for _, name := range projectContainers {
		if ctx.Err() != nil {
			break
		}
		root := filepath.Join(env.Home, name)
		identity := projectIdentity(root)
		if containers[identity] {
			continue
		}
		containers[identity] = true
		st, err := os.Stat(root)
		if errors.Is(err, os.ErrNotExist) || err == nil && !st.IsDir() {
			continue
		}
		if err != nil {
			warnings = append(warnings, "unreadable_project_search:"+key("search-", root))
			continue
		}
		f, err := os.Open(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			warnings = append(warnings, "unreadable_project_search:"+key("search-", root))
			continue
		}
		entries, readErr := f.ReadDir(remaining + 1)
		f.Close()
		if len(entries) > remaining {
			entries = entries[:remaining]
			searchLimited = true
		}
		remaining -= len(entries)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			warnings = append(warnings, "unreadable_project_search:"+key("search-", root))
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if ctx.Err() != nil {
				break
			}
			if strings.HasPrefix(entry.Name(), ".") || !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if absolutePath(path) && projectEvidence(path) {
				add(path, "project-directory")
			}
		}
	}
	if limited {
		warnings = append(warnings, "project_limit")
	}
	if searchLimited {
		warnings = append(warnings, "project_search_limit")
	}
	return projects, warnings
}
