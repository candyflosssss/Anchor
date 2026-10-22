package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/managed"
)

func TestProjectDiscoveryCollectsWithoutRegistrationOrCodexIndex(t *testing.T) {
	f := setup(t)
	project := filepath.Join(f.env.Home, "Projects", "example")
	for i, item := range projectSkillDirs {
		put(t, filepath.Join(project, item.dir, "skills", "skill-"+item.agent, "SKILL.md"), fmt.Sprint(i))
	}
	f.refresh()
	if len(f.ws.Library) != len(projectSkillDirs) || len(f.ws.Sources) != len(projectSkillDirs) || len(f.ws.Warnings) != 0 {
		t.Fatal("project skills require registration", f.ws)
	}
	for _, scope := range f.ws.Scopes {
		if scope.Kind == "project" && (scope.Project != project || scope.Origin != "archived-source") {
			t.Fatal(scope)
		}
	}
	v, err := f.m.load()
	if err != nil || len(v.Projects) != 0 {
		t.Fatal("automatic discovery changed explicit registration list", v.Projects, err)
	}
	revision := f.ws.Revision
	f.refresh()
	if f.ws.Revision != revision {
		t.Fatal("unchanged discovery rewrote the library")
	}
	// Both a new Skill in a known project and a newly created project are found.
	put(t, filepath.Join(project, ".agents", "skills", "new", "SKILL.md"), "new skill")
	second := filepath.Join(f.env.Home, "Projects", "later", ".claude", "skills", "later")
	put(t, filepath.Join(second, "SKILL.md"), "new project")
	f.refresh()
	if len(f.ws.Library) != len(projectSkillDirs)+2 {
		t.Fatal("incremental project discovery missed a new source", f.ws)
	}
	for _, source := range f.ws.Sources {
		st, err := os.Lstat(source.Path)
		if err != nil || !st.IsDir() || source.Status != "in_sync" {
			t.Fatal("automatic project collection modified an original", source, err)
		}
	}
}

func TestProjectDiscoveryOnlyInspectsKnownContainersAndDirectProjects(t *testing.T) {
	f := setup(t)
	for _, name := range projectContainers {
		put(t, filepath.Join(f.env.Home, name, "repo", ".codex", "skills", "found", "SKILL.md"), "found")
	}
	for _, path := range []string{
		filepath.Join(f.env.Home, "private", "repo"),
		filepath.Join(f.env.Home, "Projects", "group", "nested"),
		filepath.Join(f.env.Home, "Projects", ".hidden"),
	} {
		put(t, filepath.Join(path, ".agents", "skills", "hidden", "SKILL.md"), "do not collect")
	}
	// A repository's dependencies must never become separate project candidates.
	put(t, filepath.Join(f.env.Home, "Projects", "repo", "node_modules", "pkg", ".agents", "skills", "hidden", "SKILL.md"), "dependency")
	f.refresh()
	if len(f.ws.Library) != 1 {
		t.Fatal("scanned outside the project search boundary", f.ws.Library)
	}
	projects, warnings := discoverProjects(context.Background(), manifest{}, f.env)
	want := len(projectContainers)
	if runtime.GOOS == "windows" {
		want -= 4 // Projects/projects, Code/code, Work/work, Workspace/workspace.
	}
	if len(projects) != want || len(warnings) != 0 {
		t.Fatal(projects, warnings)
	}
	// Custom paths remain available as an optional supplement, not a prerequisite.
	f.act(SkillAction{Action: "register-project", ProjectPath: filepath.Join(f.env.Home, "private", "repo")})
	if len(f.ws.Library) != 2 {
		t.Fatal("explicit custom root stopped working", f.ws)
	}
}

func TestProjectDiscoveryDeduplicatesProjectAliasesAndSkillIdentities(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires native symlink privileges")
	}
	f := setup(t)
	project := filepath.Join(f.env.Home, "Projects", "a-real")
	real := filepath.Join(project, ".agents", "skills")
	put(t, filepath.Join(real, "example", "SKILL.md"), "one physical skill")
	if err := os.MkdirAll(filepath.Join(project, ".claude"), 0700); err != nil {
		t.Fatal(err)
	}
	for from, to := range map[string]string{
		filepath.Join(f.env.Home, "Projects", "b-alias"): project,
		filepath.Join(f.env.Home, "Code"):                filepath.Join(f.env.Home, "Projects"),
		filepath.Join(project, ".claude", "skills"):      real,
	} {
		if err := os.Symlink(to, from); err != nil {
			t.Fatal(err)
		}
	}
	f.refresh()
	projects, warnings := discoverProjects(context.Background(), manifest{}, f.env)
	if len(projects) != 1 || len(warnings) != 0 || len(f.ws.Library) != 1 || len(f.ws.Sources) != 2 {
		t.Fatal(projects, warnings, f.ws)
	}
	if f.ws.Sources[0].SkillID != f.ws.Sources[1].SkillID {
		t.Fatal("aliases created duplicate library entries")
	}
	// Explicit registrations take priority, even if their spelling sorts later.
	alias := filepath.Join(f.env.Home, "Projects", "b-alias")
	f.act(SkillAction{Action: "register-project", ProjectPath: alias})
	for _, source := range f.ws.Sources {
		if source.Status == "missing" {
			t.Fatal("registering an alias lost an already archived source", source)
		}
	}
	projects, _ = discoverProjects(context.Background(), manifest{Projects: []string{alias}}, f.env)
	if len(projects) != 1 || projects[alias] != "registered" {
		t.Fatal("automatic path displaced an explicit registration", projects)
	}
}

func TestProjectDiscoveryRemembersArchivedProjectsWithoutMisclassifyingGlobalSkills(t *testing.T) {
	f := setup(t)
	project := temp(t)
	put(t, filepath.Join(project, ".agents", "skills", "remembered", "SKILL.md"), "project")
	index := filepath.Join(f.env.Home, ".codex", ".codex-global-state.json")
	b, _ := json.Marshal(map[string][]string{"electron-saved-workspace-roots": {project}})
	put(t, index, string(b))
	put(t, filepath.Join(f.env.Home, ".agents", "skills", "global", "SKILL.md"), "global")
	f.env.ClaudeHome = filepath.Join(temp(t), ".claude")
	put(t, filepath.Join(f.env.ClaudeHome, "skills", "configured-global", "SKILL.md"), "configured global")
	f.refresh()
	if len(f.ws.Library) != 3 {
		t.Fatal(f.ws)
	}
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	f.refresh()
	for _, source := range f.ws.Sources {
		if source.Status != "in_sync" {
			t.Fatal("lost a known project after its workspace index disappeared", source)
		}
	}
	v, err := f.m.load()
	if err != nil {
		t.Fatal(err)
	}
	projects, _ := discoverProjects(context.Background(), v, f.env)
	if len(projects) != 1 || projects[project] != "archived-source" {
		t.Fatal("global sources misclassified as project evidence", projects)
	}
}

func TestProjectDiscoveryIndexesAndLimitsDoNotSilentlyDropProjects(t *testing.T) {
	home := temp(t)
	env := managed.Environment{Home: home}
	paths := []string{}
	for i := 0; i <= maxDiscoveredProjects; i++ {
		path := filepath.Join(home, "Projects", fmt.Sprintf("repo-%04d", i), ".agents")
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filepath.Dir(path))
	}
	// A saved project outside the conventional containers is still discovered.
	indexed := temp(t)
	index, _ := json.Marshal(map[string][]string{
		"electron-saved-workspace-roots": {indexed, indexed, "relative", filepath.VolumeName(home) + string(filepath.Separator)},
		"electron-workspace-root-order":  {indexed},
	})
	put(t, filepath.Join(home, ".codex", ".codex-global-state.json"), string(index))
	projects, warnings := discoverProjects(context.Background(), manifest{}, env)
	if len(projects) != maxDiscoveredProjects || projects[indexed] != "codex-workspace-index" || !slices.Contains(warnings, "project_limit") {
		t.Fatal("project limit silently truncated or displaced the index", len(projects), warnings)
	}
	if projects[paths[70]] == "" {
		t.Fatal("legacy 64-project cap still blocks automatic discovery")
	}
	// Duplicate entries in both index arrays do not exhaust its project budget.
	duplicates := make([]string, maxDiscoveredProjects+1)
	for i := range duplicates {
		duplicates[i] = indexed
	}
	index, _ = json.Marshal(map[string][]string{"electron-saved-workspace-roots": duplicates, "electron-workspace-root-order": paths[:70]})
	put(t, filepath.Join(home, ".codex", ".codex-global-state.json"), string(index))
	if got := codexProjects(filepath.Join(home, ".codex")); len(got) != 71 {
		t.Fatal("duplicate indexed roots consumed the discovery budget", len(got))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if projects, warnings = discoverProjects(ctx, manifest{}, env); len(projects) != 0 || len(warnings) != 0 {
		t.Fatal("cancelled discovery continued scanning", projects, warnings)
	}
}

func TestProjectSearchEntryLimitIsReported(t *testing.T) {
	home := temp(t)
	for i := 0; i <= maxProjectSearchEntries; i++ {
		put(t, filepath.Join(home, "Projects", fmt.Sprintf("file-%04d", i)), "not a project")
	}
	projects, warnings := discoverProjects(context.Background(), manifest{}, managed.Environment{Home: home})
	if len(projects) != 0 || !slices.Contains(warnings, "project_search_limit") {
		t.Fatal(projects, warnings)
	}
}

func TestProjectSearchUnreadableContainerDoesNotBlockOthers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory permission check")
	}
	f := setup(t)
	blocked := filepath.Join(f.env.Home, "Projects")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0700) })
	if probe, err := os.Open(blocked); err == nil {
		probe.Close()
		t.Skip("current account bypasses directory permissions")
	}
	put(t, filepath.Join(f.env.Home, "Code", "readable", ".agents", "skills", "found", "SKILL.md"), "found")
	f.refresh()
	if len(f.ws.Library) != 1 || !slices.ContainsFunc(f.ws.Warnings, func(w string) bool { return strings.HasPrefix(w, "unreadable_project_search:") }) {
		t.Fatal("unreadable container hid other projects or its warning", f.ws)
	}
}

func TestProjectDiscoveryRecognizesGitWorktreesBeforeTheyHaveSkills(t *testing.T) {
	f := setup(t)
	project := filepath.Join(f.env.Home, "Projects", "worktree")
	put(t, filepath.Join(project, ".git"), "gitdir: /do-not-follow-or-read\n")
	f.refresh()
	count := 0
	for _, scope := range f.ws.Scopes {
		if scope.Project == project {
			count++
		}
	}
	if count != len(projectSkillDirs) || len(f.ws.Library) != 0 {
		t.Fatal("worktree did not become an automatic project", f.ws)
	}
	put(t, filepath.Join(project, ".codex", "skills", "new", "SKILL.md"), "later")
	f.refresh()
	if len(f.ws.Library) != 1 {
		t.Fatal("later skill in a worktree was not collected", f.ws)
	}
}
