package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	}
	if json.Unmarshal(b, &f) != nil || f.Contract != Contract || len(f.Workspace.Library) != 1 || len(f.Workspace.Sources) != 1 || len(f.Detail.Files) != 1 || f.Create.Action != "create" {
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
