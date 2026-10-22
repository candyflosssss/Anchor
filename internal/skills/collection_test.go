package skills

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestCollectionWritableSourcesAndIncrementalDiscovery(t *testing.T) {
	f := setup(t)
	root := filepath.Join(f.env.Home, ".agents", "skills", "中文 Skill")
	put(t, filepath.Join(root, "SKILL.md"), "first")
	put(t, filepath.Join(root, "docs", "guide.md"), "guide")
	if runtime.GOOS != "windows" {
		for _, path := range []string{root, filepath.Join(root, "docs")} {
			if err := os.Chmod(path, 0775); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chmod(filepath.Join(root, "docs", "guide.md"), 0664); err != nil {
			t.Fatal(err)
		}
	}
	f.refresh()
	if len(f.ws.Library) != 1 || f.ws.Sources[0].Status != "in_sync" {
		t.Fatal(f.ws)
	}
	revision := f.ws.Revision
	f.refresh()
	if revision != f.ws.Revision {
		t.Fatal("non-idempotent scan")
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(root)
		if st.Mode().Perm() != 0775 {
			t.Fatal("source permissions changed")
		}
	}
	put(t, filepath.Join(f.env.Home, ".codex", "skills", ".system", "builtin", "SKILL.md"), "system")
	f.refresh()
	if len(f.ws.Library) != 2 {
		t.Fatal("new system skill not collected", f.ws)
	}
	project := temp(t)
	put(t, filepath.Join(project, ".codex", "skills", "project-skill", "SKILL.md"), "project")
	f.act(SkillAction{Action: "register-project", ProjectPath: project})
	if len(f.ws.Library) != 3 {
		t.Fatal("registered project not collected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.m.WorkspaceContext(ctx, f.env); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCollectionAliasesResourcesAndFailedSources(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires native symlink privileges")
	}
	f := setup(t)
	root := filepath.Join(f.env.Home, ".agents", "skills")
	real := filepath.Join(temp(t), "real-skill")
	put(t, filepath.Join(real, "SKILL.md"), "original")
	resource := filepath.Join(temp(t), "resource.md")
	put(t, resource, "resource bytes")
	if err := os.Symlink(resource, filepath.Join(real, "reference.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alias-a", "alias-b"} {
		if err := os.Symlink(real, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	// Several failed sources used to exhaust a shared hash budget.
	for i := 0; i < 6; i++ {
		bad := filepath.Join(root, fmt.Sprintf("bad-%d", i))
		put(t, filepath.Join(bad, "SKILL.md"), "cycle")
		if err := os.Symlink(bad, filepath.Join(bad, "loop")); err != nil {
			t.Fatal(err)
		}
	}
	put(t, filepath.Join(root, "z-small", "SKILL.md"), "small")
	f.refresh()
	if len(f.ws.Library) != 2 {
		t.Fatal(f.ws)
	}
	var id string
	for _, s := range f.ws.Library {
		if s.Name == "real-skill" {
			id = s.ID
		}
	}
	if id == "" || content(t, filepath.Join(f.m.versionPath(id, 1), "reference.md")) != "resource bytes" {
		t.Fatal("linked resource lost")
	}
	if st, _ := os.Lstat(filepath.Join(f.m.versionPath(id, 1), "reference.md")); !st.Mode().IsRegular() {
		t.Fatal("snapshot depends on source link")
	}
	put(t, filepath.Join(real, "SKILL.md"), "updated")
	if err := os.Symlink(real, filepath.Join(root, "alias-c")); err != nil {
		t.Fatal(err)
	}
	f.refresh()
	for _, src := range f.ws.Sources {
		if src.Name == "alias-c" && (src.SkillID != id || src.Baseline != 2) {
			t.Fatal("alias associated with stale content", src)
		}
	}
}

func TestDeletedSourcesStayExcludedUntilExplicitImport(t *testing.T) {
	f := setup(t)
	root := filepath.Join(f.env.Home, ".agents", "skills", "sample")
	put(t, filepath.Join(root, "SKILL.md"), "source")
	f.refresh()
	f.act(SkillAction{Action: "delete", SkillID: f.ws.Library[0].ID})
	f.refresh()
	if len(f.ws.Library) != 0 || f.ws.Sources[0].Status != "excluded" {
		t.Fatal(f.ws)
	}
	src := f.ws.Sources[0]
	f.act(SkillAction{Action: "import", SourceID: src.ID, ExpectedHash: src.Hash})
	if len(f.ws.Library) != 1 || f.ws.Sources[0].Status != "in_sync" {
		t.Fatal(f.ws)
	}
}

func TestCleanupArchivesLatestAndOnlyReplacesApprovedSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires native symlink privileges")
	}
	for _, alias := range []bool{false, true} {
		t.Run(fmt.Sprint(alias), func(t *testing.T) {
			f := setup(t)
			source := filepath.Join(f.env.Home, ".agents", "skills", "sample")
			real := source
			if alias {
				real = filepath.Join(temp(t), "sample")
			}
			put(t, filepath.Join(real, "SKILL.md"), "first")
			put(t, filepath.Join(real, "scripts", "run.sh"), "#!/bin/sh\n")
			if err := os.Chmod(filepath.Join(real, "scripts", "run.sh"), 0755); err != nil {
				t.Fatal(err)
			}
			if alias {
				if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, source); err != nil {
					t.Fatal(err)
				}
			}
			f.refresh()
			old := f.ws.Sources[0]
			put(t, filepath.Join(real, "SKILL.md"), "latest")
			f.refresh()
			in := SkillAction{Action: "cleanup", SourceID: old.ID, ExpectedHash: old.Hash, ExpectedRevision: f.ws.Revision}
			if _, err := f.m.Action(f.env, in); !errors.Is(err, state.ErrInvalid) {
				t.Fatal(err)
			}
			in.Approved = true
			if _, err := f.m.Action(f.env, in); !errors.Is(err, ErrChanged) {
				t.Fatal(err)
			}
			f.act(SkillAction{Action: "cleanup", SourceID: old.ID, ExpectedHash: f.ws.Sources[0].Hash})
			if len(f.ws.Mounts) != 1 || len(f.ws.Sources) != 0 || len(f.ws.Library[0].Versions) != 2 {
				t.Fatal(f.ws)
			}
			requireLink(t, source, f.m.versionPath(old.SkillID, 2))
			if content(t, filepath.Join(source, "SKILL.md")) != "latest" {
				t.Fatal("latest bytes lost")
			}
			if alias && content(t, filepath.Join(real, "SKILL.md")) != "latest" {
				t.Fatal("alias target deleted")
			}
			st, _ := os.Stat(filepath.Join(source, "scripts", "run.sh"))
			if !executable(st.Mode()) {
				t.Fatal("executable flag lost")
			}
			entries, _ := filepath.Glob(filepath.Join(filepath.Dir(source), ".anchor-skills-cleanup-*"))
			if len(entries) != 0 {
				t.Fatal("backup not removed", entries)
			}
		})
	}
}

func TestCleanupRecoveryAndConflictingChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires native symlink privileges")
	}
	for _, phase := range []string{"prepared", "moved", "linked", "published", "changed", "conflict"} {
		t.Run(phase, func(t *testing.T) {
			f := setup(t)
			path := filepath.Join(f.env.Home, ".agents", "skills", "sample")
			put(t, filepath.Join(path, "SKILL.md"), "source")
			f.refresh()
			src := f.ws.Sources[0]
			v, err := f.m.load()
			if err != nil {
				t.Fatal(err)
			}
			p := &cleanupIntent{Mount: SkillMount{ID: key("mount-", path), SkillID: src.SkillID, Version: 1, ScopeID: src.ScopeID, Path: path, Target: f.m.versionPath(src.SkillID, 1), Status: "linked"}, Hash: src.Hash, Backup: filepath.Join(filepath.Dir(path), ".anchor-skills-cleanup-"+randomID())}
			v.Cleanup = p
			if phase != "prepared" {
				if err := os.Rename(path, p.Backup); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "linked" || phase == "published" {
				if err := os.Symlink(p.Mount.Target, path); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "published" {
				p.Published = true
				v.Mounts = append(v.Mounts, p.Mount)
				v.Sources = []association{}
			}
			if phase == "changed" {
				put(t, filepath.Join(p.Backup, "SKILL.md"), "changed")
			}
			if phase == "conflict" {
				put(t, filepath.Join(path, "SKILL.md"), "foreign")
			}
			if err := f.m.persist(v); err != nil {
				t.Fatal(err)
			}
			f.m = New(filepath.Dir(f.m.root))
			_, err = f.m.Action(f.env, SkillAction{Action: "register-project", ProjectPath: temp(t), ExpectedRevision: v.Revision, Approved: true})
			if phase == "changed" || phase == "conflict" {
				if !errors.Is(err, ErrChanged) {
					t.Fatal(err)
				}
				want := "changed"
				if phase == "conflict" {
					want = "foreign"
				}
				if content(t, filepath.Join(path, "SKILL.md")) != want {
					t.Fatal("external changes discarded")
				}
				if phase == "conflict" && content(t, filepath.Join(p.Backup, "SKILL.md")) != "source" {
					t.Fatal("backup discarded")
				}
				return
			}
			// The triggering operation must not run with a pre-recovery revision.
			if phase != "published" && !errors.Is(err, state.ErrConflict) {
				t.Fatal(err)
			}
			if phase == "published" && err != nil {
				t.Fatal(err)
			}
			f.refresh()
			requireLink(t, path, p.Mount.Target)
			if len(f.ws.Warnings) != 0 || len(f.ws.Mounts) != 1 || !absent(p.Backup) {
				t.Fatal(f.ws)
			}
		})
	}
}

func TestCleanupAliasAfterItsTargetWasCleaned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires native symlink privileges")
	}
	f := setup(t)
	path := filepath.Join(f.env.Home, ".agents", "skills", "sample")
	alias := filepath.Join(f.env.Home, ".agents", "skills", "alias")
	put(t, filepath.Join(path, "SKILL.md"), "source")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	f.refresh()
	for _, source := range []string{path, alias} {
		var src SkillSource
		for _, s := range f.ws.Sources {
			if s.Path == source {
				src = s
			}
		}
		if src.ID == "" {
			t.Fatal("source missing", source)
		}
		f.act(SkillAction{Action: "cleanup", SourceID: src.ID, ExpectedHash: src.Hash})
	}
	if len(f.ws.Library) != 1 || len(f.ws.Mounts) != 2 || len(f.ws.Sources) != 0 {
		t.Fatal(f.ws)
	}
	if content(t, filepath.Join(alias, "SKILL.md")) != "source" {
		t.Fatal("snapshot damaged")
	}
	if err := os.Symlink(f.ws.Mounts[0].Target, filepath.Join(filepath.Dir(alias), "new-alias")); err != nil {
		t.Fatal(err)
	}
	f.refresh()
	if len(f.ws.Library) != 1 || len(f.ws.Sources) != 1 || f.ws.Sources[0].SkillID != f.ws.Library[0].ID {
		t.Fatal("snapshot link duplicated", f.ws)
	}
}

func TestCollectionDoesNotHideSnapshotCorruptionWithDuplicateIdentity(t *testing.T) {
	f := setup(t)
	put(t, filepath.Join(f.env.Home, ".agents", "skills", "sample", "SKILL.md"), "source")
	f.refresh()
	snapshot := filepath.Join(f.m.versionPath(f.ws.Library[0].ID, 1), "SKILL.md")
	if err := os.Chmod(snapshot, 0600); err != nil {
		t.Fatal(err)
	}
	put(t, snapshot, "tampered")
	put(t, filepath.Join(f.env.Home, ".codex", "skills", "sample", "SKILL.md"), "source")
	f.refresh()
	if len(f.ws.Library) != 1 || len(f.ws.Warnings) == 0 {
		t.Fatal(f.ws)
	}
}
