package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/state"
)

type fixture struct {
	m   *Manager
	env managed.Environment
	ws  SkillWorkspace
	t   *testing.T
}

func setup(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{m: New(temp(t)), env: managed.Environment{Home: temp(t)}, t: t}
	var err error
	f.ws, err = f.m.Workspace(f.env)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *fixture) act(in SkillAction) SkillWorkspace {
	f.t.Helper()
	in.Approved = true
	in.ExpectedRevision = f.ws.Revision
	v, err := f.m.Action(f.env, in)
	if err != nil {
		f.t.Fatal(in.Action, err)
	}
	f.ws = v
	return v
}
func (f *fixture) refresh() {
	f.t.Helper()
	var err error
	f.ws, err = f.m.Workspace(f.env)
	if err != nil {
		f.t.Fatal(err)
	}
}
func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func content(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func edit(p, s string) []SkillEdit { return []SkillEdit{{Path: p, Content: s}} }
func requireLink(t *testing.T, path, to string) {
	t.Helper()
	got, err := os.Readlink(path)
	if err != nil || got != to {
		t.Fatal(path, got, err)
	}
}
func TestReadOnlyInventoryAndCopyImport(t *testing.T) {
	f := setup(t)
	if _, err := os.Stat(f.m.root); !os.IsNotExist(err) {
		t.Fatal("read created state", err)
	}
	project := temp(t)
	original := filepath.Join(project, ".agents", "skills", "example")
	put(t, filepath.Join(original, "SKILL.md"), "old\n")
	put(t, filepath.Join(original, "scripts", "run.sh"), "#!/bin/sh\nexit 0\n")
	if runtime.GOOS != "windows" {
		_ = os.Chmod(filepath.Join(original, "scripts", "run.sh"), 0700)
	}
	before, _ := os.Lstat(original)
	f.act(SkillAction{Action: "register-project", ProjectPath: project})
	if len(f.ws.Sources) != 1 {
		t.Fatal(f.ws)
	}
	src := f.ws.Sources[0]
	f.act(SkillAction{Action: "import", SourceID: src.ID, ExpectedHash: src.Hash})
	if len(f.ws.Library) != 1 || len(f.ws.Library[0].Versions) != 1 {
		t.Fatal(f.ws.Library)
	}
	after, _ := os.Lstat(original)
	if !os.SameFile(before, after) || after.Mode()&os.ModeSymlink != 0 {
		t.Fatal("original replaced")
	}
	s := f.ws.Library[0]
	if content(t, f.m.versionPath(s.ID, 1)+"/scripts/run.sh") != "#!/bin/sh\nexit 0\n" {
		t.Fatal("resources lost")
	}
	f.act(SkillAction{Action: "import", SourceID: src.ID, ExpectedHash: src.Hash})
	if len(f.ws.Library[0].Versions) != 1 {
		t.Fatal("duplicate version")
	}
	if _, err := f.m.Action(f.env, SkillAction{Action: "link", ExpectedRevision: f.ws.Revision, Approved: true, SkillID: s.ID, Version: 1, ScopeID: src.ScopeID}); !errors.Is(err, state.ErrConflict) {
		t.Fatal("original overwritten", err)
	}
	if content(t, filepath.Join(original, "SKILL.md")) != "old\n" {
		t.Fatal("source modified")
	}
}
func TestBaselineDivergenceAndExplicitVersions(t *testing.T) {
	f := setup(t)
	root := filepath.Join(f.env.Home, ".agents", "skills", "sample")
	put(t, filepath.Join(root, "SKILL.md"), "base\n")
	f.refresh()
	src := f.ws.Sources[0]
	f.act(SkillAction{Action: "import", SourceID: src.ID, ExpectedHash: src.Hash})
	id := f.ws.Library[0].ID
	put(t, filepath.Join(root, "SKILL.md"), "source edit\n")
	f.refresh()
	if f.ws.Sources[0].Status != "source_changed" || len(f.ws.Library[0].Versions) != 1 {
		t.Fatal(f.ws)
	}
	f.act(SkillAction{Action: "save", SkillID: id, Version: 1, Edits: edit("SKILL.md", "library edit\n")})
	if f.ws.Sources[0].Status != "diverged" {
		t.Fatal(f.ws.Sources)
	}
	d, err := f.m.Detail(f.env, SkillQuery{SourceID: src.ID})
	if err != nil || len(d.Differences) != 1 || d.Differences[0].Before != "base\n" {
		t.Fatal(d, err)
	}
	d, err = f.m.Detail(f.env, SkillQuery{SourceID: src.ID, AgainstVersion: 2})
	if err != nil || d.Differences[0].Before != "library edit\n" {
		t.Fatal(d, err)
	}
	if _, err = f.m.Action(f.env, SkillAction{Action: "import", ExpectedRevision: f.ws.Revision, Approved: true, SourceID: src.ID, ExpectedHash: src.Hash}); !errors.Is(err, ErrChanged) {
		t.Fatal("stale import", err)
	}
	f.act(SkillAction{Action: "import", SourceID: src.ID, ExpectedHash: f.ws.Sources[0].Hash})
	versions := f.ws.Library[0].Versions
	if len(versions) != 3 || versions[2].Parent != 2 || versions[2].Baseline != 1 {
		t.Fatal(versions)
	}
	if content(t, f.m.versionPath(id, 1)+"/SKILL.md") != "base\n" || content(t, f.m.versionPath(id, 2)+"/SKILL.md") != "library edit\n" {
		t.Fatal("immutable version changed")
	}
	put(t, filepath.Join(root, "SKILL.md"), "library edit\n")
	f.refresh()
	f.act(SkillAction{Action: "import", SourceID: src.ID, ExpectedHash: f.ws.Sources[0].Hash})
	if len(f.ws.Library[0].Versions) != 3 || f.ws.Sources[0].Baseline != 2 {
		t.Fatal("identical content not reused")
	}
}
func TestPinnedMountsExplicitUpdateAndConflictProtection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires native symbolic link privileges")
	}
	f := setup(t)
	f.act(SkillAction{Action: "create", Name: "new-skill", Edits: edit("SKILL.md", "one")})
	id := f.ws.Library[0].ID
	scope := f.ws.Scopes[0]
	f.act(SkillAction{Action: "link", SkillID: id, Version: 1, ScopeID: scope.ID})
	mt := f.ws.Mounts[0]
	requireLink(t, mt.Path, f.m.versionPath(id, 1))
	f.act(SkillAction{Action: "save", SkillID: id, Version: 1, Edits: edit("SKILL.md", "two")})
	requireLink(t, mt.Path, f.m.versionPath(id, 1))
	f.act(SkillAction{Action: "update", MountID: mt.ID, Version: 2})
	requireLink(t, mt.Path, f.m.versionPath(id, 2))
	f.act(SkillAction{Action: "update", MountID: mt.ID, Version: 1})
	requireLink(t, mt.Path, f.m.versionPath(id, 1))
	if _, err := f.m.Action(f.env, SkillAction{Action: "delete", ExpectedRevision: f.ws.Revision, Approved: true, SkillID: id}); !errors.Is(err, state.ErrReferenced) {
		t.Fatal("deleted referenced skill", err)
	}
	_ = os.Remove(mt.Path)
	put(t, filepath.Join(mt.Path, "SKILL.md"), "external")
	if _, err := f.m.Action(f.env, SkillAction{Action: "unlink", ExpectedRevision: f.ws.Revision, Approved: true, MountID: mt.ID}); !errors.Is(err, ErrChanged) {
		t.Fatal("external entry removed", err)
	}
	if content(t, filepath.Join(mt.Path, "SKILL.md")) != "external" {
		t.Fatal("external changed")
	}
	_ = os.Remove(filepath.Join(mt.Path, "SKILL.md"))
	_ = os.Remove(mt.Path)
	f.refresh()
	if f.ws.Mounts[0].Status != "missing" || !absent(mt.Path) {
		t.Fatal("automatically recreated mount")
	}
	// An absent link is forgotten only by an explicit unlink action.
	f.act(SkillAction{Action: "unlink", MountID: mt.ID})
	if len(f.ws.Mounts) != 0 {
		t.Fatal(f.ws.Mounts)
	}
	f.act(SkillAction{Action: "delete", SkillID: id})
	if len(f.ws.Library) != 0 {
		t.Fatal(f.ws.Library)
	}
}
func TestUnsafeTreesAndScopePaths(t *testing.T) {
	f := setup(t)
	for _, path := range []string{"../outside", "/outside", "scripts/../../outside", "scripts\\outside", "NUL", "SKILL.md/child"} {
		_, err := f.m.Action(f.env, SkillAction{Action: "create", Name: "safe", ExpectedRevision: f.ws.Revision, Approved: true, Edits: append(edit("SKILL.md", "base"), SkillEdit{Path: path, Content: "bad"})})
		if err == nil {
			t.Fatal("unsafe path accepted", path)
		}
	}
	outside := temp(t)
	put(t, filepath.Join(outside, "SKILL.md"), "private")
	if runtime.GOOS != "windows" {
		root := filepath.Join(f.env.Home, ".agents", "skills")
		_ = os.MkdirAll(root, 0700)
		_ = os.Symlink(outside, filepath.Join(root, "linked"))
		f.refresh()
		if f.ws.Sources[0].Status != "external_link" {
			t.Fatal(f.ws)
		}
		_, err := f.m.Action(f.env, SkillAction{Action: "import", SourceID: f.ws.Sources[0].ID, ExpectedHash: "no", ExpectedRevision: f.ws.Revision, Approved: true})
		if err == nil {
			t.Fatal("followed external symlink")
		}
	}
}
func TestSnapshotIntegrityAndExecutableDifference(t *testing.T) {
	f := setup(t)
	f.act(SkillAction{Action: "create", Name: "test", Edits: edit("SKILL.md", "base")})
	s := f.ws.Library[0]
	p := filepath.Join(f.m.versionPath(s.ID, 1), "SKILL.md")
	_ = os.Chmod(p, 0600)
	put(t, p, "tampered")
	_, err := f.m.Detail(f.env, SkillQuery{SkillID: s.ID, Version: 1})
	if !errors.Is(err, ErrChanged) {
		t.Fatal(err)
	}
	a := tree{Files: []SkillFile{{Path: "SKILL.md", Hash: "same"}}, Data: map[string][]byte{"SKILL.md": []byte("x")}}
	b := a
	b.Files = []SkillFile{{Path: "SKILL.md", Hash: "same", Executable: true}}
	if len(differences(a, b)) != 1 {
		t.Fatal("permission-only change ignored")
	}
}
func TestSameNameSeparateIdentityAndReadLimits(t *testing.T) {
	f := setup(t)
	for _, name := range []string{"same", "same"} {
		f.act(SkillAction{Action: "create", Name: name, Edits: edit("SKILL.md", "x")})
	}
	if f.ws.Library[0].ID == f.ws.Library[1].ID {
		t.Fatal("name used as ID")
	}
	_, err := edited(tree{Data: map[string][]byte{}, Files: []SkillFile{}}, edit("SKILL.md", strings.Repeat("x", MaxText+1)))
	if err == nil {
		t.Fatal("text size unbounded")
	}
}

func temp(t *testing.T) string {
	t.Helper()
	p := t.TempDir()
	if err := os.Chmod(p, 0700); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUntrustedManifestRejected(t *testing.T) {
	f := setup(t)
	f.act(SkillAction{Action: "create", Name: "demo", Edits: edit("SKILL.md", "content")})
	v, err := f.m.load()
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*manifest){
		"id traversal":   func(v *manifest) { v.Library[0].ID = "../../outside" },
		"empty versions": func(v *manifest) { v.Library[0].Versions = nil },
		"file traversal": func(v *manifest) { v.Library[0].Versions[0].Files[0].Path = "../outside" },
		"forged hash":    func(v *manifest) { v.Library[0].Versions[0].Hash = strings.Repeat("0", 64) },
		"forged mount": func(v *manifest) {
			v.Mounts = []SkillMount{{ID: "foreign", SkillID: v.Library[0].ID, Version: 1, Path: filepath.Join(f.env.Home, "external"), Target: f.env.Home}}
		},
		"empty journal": func(v *manifest) { v.Pending = &intent{} },
	} {
		t.Run(name, func(t *testing.T) {
			b, _ := json.Marshal(v)
			var bad manifest
			_ = json.Unmarshal(b, &bad)
			mutate(&bad)
			b, _ = json.Marshal(bad)
			put(t, filepath.Join(f.m.root, "index.json"), string(b))
			if _, err := f.m.Workspace(f.env); !errors.Is(err, ErrUnsafe) {
				t.Fatal(err)
			}
		})
	}
}

func TestPendingMountRecoveryNeverOverwritesForeignEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Windows symlink permission not available")
	}
	for _, published := range []bool{false, true} {
		t.Run(fmt.Sprint(published), func(t *testing.T) {
			f := setup(t)
			f.act(SkillAction{Action: "create", Name: "demo", Edits: edit("SKILL.md", "one")})
			s := f.ws.Library[0]
			f.act(SkillAction{Action: "save", SkillID: s.ID, Version: 1, Edits: edit("SKILL.md", "two")})
			f.act(SkillAction{Action: "link", SkillID: s.ID, Version: 1, ScopeID: f.ws.Scopes[0].ID})
			v, _ := f.m.load()
			before := v.Mounts[0]
			after := before
			after.Version, after.Target = 2, f.m.versionPath(s.ID, 2)
			backup := filepath.Join(filepath.Dir(before.Path), ".anchor-skills-"+randomID())
			v.Pending = &intent{Before: &before, After: &after, Backup: backup}
			if err := f.m.persist(v); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(before.Path, backup); err != nil {
				t.Fatal(err)
			}
			if published {
				if err := os.Symlink(after.Target, after.Path); err != nil {
					t.Fatal(err)
				}
			} else {
				put(t, before.Path, "foreign")
				_, err := f.m.Action(f.env, SkillAction{Action: "register-project", ProjectPath: temp(t), ExpectedRevision: v.Revision, Approved: true})
				if !errors.Is(err, ErrChanged) || content(t, before.Path) != "foreign" {
					t.Fatal(err)
				}
				requireLink(t, backup, before.Target)
				if err := os.Remove(before.Path); err != nil {
					t.Fatal(err)
				}
			}
			// Recovery commits only the prior approved change. The triggering stale
			// action is rejected, so it cannot accidentally run against a new revision.
			_, err := f.m.Action(f.env, SkillAction{Action: "register-project", ProjectPath: temp(t), ExpectedRevision: v.Revision, Approved: true})
			if !errors.Is(err, state.ErrConflict) {
				t.Fatal(err)
			}
			f.refresh()
			if f.ws.Mounts[0].Version != 2 || len(f.ws.Warnings) != 0 {
				t.Fatal(f.ws)
			}
			requireLink(t, before.Path, after.Target)
			if !absent(backup) {
				t.Fatal("backup not cleaned")
			}
		})
	}
}

func TestCaseCollisionAndDiffBudget(t *testing.T) {
	if runtime.GOOS != "windows" {
		dir := temp(t)
		put(t, filepath.Join(dir, "SKILL.md"), "content")
		put(t, filepath.Join(dir, "Skill.md"), "other")
		if _, err := readTree(dir); !errors.Is(err, ErrUnsafe) {
			t.Fatal(err)
		}
	}
	a := tree{Files: []SkillFile{}, Data: map[string][]byte{}}
	b := tree{Files: []SkillFile{}, Data: map[string][]byte{}}
	for i := 0; i < MaxFiles; i++ {
		p := fmt.Sprintf("file-%d.md", i)
		data := []byte(strings.Repeat("text", 1024))
		b.Files = append(b.Files, SkillFile{Path: p, Hash: digest(data), Size: int64(len(data))})
		b.Data[p] = data
	}
	preview := 0
	truncated := false
	for _, d := range differences(a, b) {
		preview += len(d.Before) + len(d.After)
		truncated = truncated || d.Truncated
	}
	if preview > 128<<10 || !truncated {
		t.Fatal(preview, truncated)
	}
}
