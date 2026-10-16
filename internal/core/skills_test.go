package core

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/skills"
)

func TestSkillManagementIsIdentityGuardedAndIsolated(t *testing.T) {
	f := setup(t)
	home := t.TempDir()
	_ = os.Chmod(home, 0700)
	f.core.AgentEnvironment = func() (managed.Environment, error) { return managed.Environment{Home: home}, nil }
	body := skills.SkillAction{Action: "create", ExpectedRevision: 1, Approved: true, Name: "demo", Edits: []skills.SkillEdit{{Path: "SKILL.md", Content: "synthetic"}}}
	denied := controlWrite(t, f, "/control/v1/skills/action", body, false)
	if denied.StatusCode != 409 {
		t.Fatal(denied.StatusCode, text(t, denied))
	}
	denied.Body.Close()
	for _, token := range []string{"", "not-owner"} {
		r := request(t, "GET", f.control.URL+"/control/v1/skills", token, "")
		if r.StatusCode != http.StatusUnauthorized {
			t.Fatal(r.StatusCode)
		}
		r.Body.Close()
	}
	created := controlWrite(t, f, "/control/v1/skills/action", body, true)
	var ws skills.SkillWorkspace
	if json.NewDecoder(created.Body).Decode(&ws) != nil || created.StatusCode != 200 || len(ws.Library) != 1 {
		t.Fatal(ws, created.StatusCode)
	}
	created.Body.Close()
	stale := controlWrite(t, f, "/control/v1/skills/action", body, true)
	if stale.StatusCode != 409 {
		t.Fatal(stale.StatusCode)
	}
	stale.Body.Close()
	detail := controlWrite(t, f, "/control/v1/skills/detail", skills.SkillQuery{SkillID: ws.Library[0].ID, Version: 1, File: "SKILL.md"}, true)
	var d skills.SkillDetail
	_ = json.NewDecoder(detail.Body).Decode(&d)
	detail.Body.Close()
	if d.Content != "synthetic" {
		t.Fatal(d)
	}
}
