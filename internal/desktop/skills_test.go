package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSharedSkillContract(t *testing.T) {
	b, err := os.ReadFile("../../desktop/contract/skills.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Contract  string         `json:"contract"`
		Workspace SkillWorkspace `json:"workspace"`
		Detail    SkillDetail    `json:"detail"`
		Create    SkillAction    `json:"create"`
		Cleanup   SkillAction    `json:"cleanup"`
	}
	if json.Unmarshal(b, &f) != nil || f.Contract != Contract || len(f.Workspace.Library) != 1 || len(f.Workspace.Sources) != 1 || len(f.Detail.Files) != 1 || f.Create.Action != "create" || f.Cleanup.Action != "cleanup" || f.Cleanup.ExpectedHash != f.Workspace.Sources[0].Hash {
		t.Fatal("skill fixture drift")
	}
	again, _ := json.Marshal(f)
	var x, y any
	_ = json.Unmarshal(b, &x)
	_ = json.Unmarshal(again, &y)
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	if string(xb) != string(yb) {
		t.Fatal("skill field drift")
	}
}
func TestRealCoreSkillBridge(t *testing.T) {
	home := t.TempDir()
	_ = os.Chmod(home, 0700)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("DSH_HOME", filepath.Join(home, ".dsh"))
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	first := b.SkillWorkspace()
	if first.Problem != nil || first.Data == nil || len(first.Data.Library) != 0 {
		t.Fatal(first)
	}
	r := b.ManageSkill(SkillAction{Action: "create", ExpectedRevision: first.Data.Revision, Approved: true, Name: "example", Edits: []SkillEdit{{Path: "SKILL.md", Content: "# Synthetic\n"}}})
	if r.Problem != nil || r.Data == nil || len(r.Data.Library) != 1 {
		t.Fatal(r)
	}
	d := b.SkillDetail(SkillQuery{SkillID: r.Data.Library[0].ID, Version: 1, File: "SKILL.md"})
	if d.Problem != nil || d.Data == nil || d.Data.Content != "# Synthetic\n" {
		t.Fatal(d)
	}
	stale := b.ManageSkill(SkillAction{Action: "create", ExpectedRevision: 1, Approved: true, Name: "stale", Edits: []SkillEdit{{Path: "SKILL.md", Content: "x"}}})
	if stale.Problem == nil || stale.Problem.Code != "conflict" {
		t.Fatal(stale)
	}
	unapproved := b.ManageSkill(SkillAction{Action: "delete", ExpectedRevision: r.Data.Revision, SkillID: r.Data.Library[0].ID})
	if unapproved.Problem == nil || unapproved.Problem.Code != "invalid" {
		t.Fatal(unapproved)
	}
}

func TestRealCoreSkillCollectionAndCleanupBridge(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native symlink privileges required")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("DSH_HOME", filepath.Join(home, ".dsh"))
	source := filepath.Join(home, ".agents", "skills", "external")
	if err := os.MkdirAll(source, 0775); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("# External\n"), 0664); err != nil {
		t.Fatal(err)
	}
	b := NewBridge(testSession(t))
	requireReady(t, b)
	first := b.SkillWorkspace()
	if first.Problem != nil || first.Data == nil || len(first.Data.Library) != 1 || len(first.Data.Sources) != 1 {
		t.Fatal(first)
	}
	src := first.Data.Sources[0]
	bad := b.ManageSkill(SkillAction{Action: "cleanup", ExpectedRevision: first.Data.Revision, Approved: true, SourceID: src.ID, ExpectedHash: "stale"})
	if bad.Problem == nil || bad.Problem.Code != "skill_changed" {
		t.Fatal(bad)
	}
	cleaned := b.ManageSkill(SkillAction{Action: "cleanup", ExpectedRevision: first.Data.Revision, Approved: true, SourceID: src.ID, ExpectedHash: src.Hash})
	if cleaned.Problem != nil || cleaned.Data == nil || len(cleaned.Data.Mounts) != 1 || len(cleaned.Data.Sources) != 0 {
		t.Fatal(cleaned)
	}
	if link, err := os.Readlink(source); err != nil || link != cleaned.Data.Mounts[0].Target {
		t.Fatal(link, err)
	}
}
