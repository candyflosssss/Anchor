package desktop

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestPrioritySharedWorkspaceContract(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/provider-workspace.json")
	if err != nil {
		t.Fatal(err)
	}
	var w Workspace
	if err = json.Unmarshal(raw, &w); err != nil || w.Revision != 3 || len(w.Keys) != 1 || w.Prices == nil || w.Estimates == nil {
		t.Fatal(w, err)
	}
	encoded, _ := json.Marshal(w)
	var expected, actual any
	json.Unmarshal(raw, &expected)
	json.Unmarshal(encoded, &actual) // check no omitted/new zero fields
	a, _ := json.Marshal(actual)
	b, _ := json.Marshal(expected)
	if string(a) != string(b) {
		t.Fatal("workspace fixture drift")
	}
}
func TestPriorityWorkspaceBridgeReadWrite(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	secret := "synthetic-workspace-key"
	p := b.SaveProvider(SaveProvider{Provider: Provider{ID: "p", BaseURL: "http://127.0.0.1:1/v1", Protocol: "anthropic", Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 1}, Secret: &secret})
	if p.Problem != nil || p.Data == nil {
		t.Fatal(p)
	}
	r := b.ProviderWorkspace(WorkspaceQuery{ProviderID: "p"})
	if r.Problem != nil || r.Data == nil || len(r.Data.Keys) != 1 {
		t.Fatal(r)
	}
	result := b.ProviderAction(Action{ProviderID: "p", ExpectedRevision: 1, Action: "key-put", Target: "second", Secret: "synthetic-second", Enabled: true})
	if result.Problem != nil || result.Data == nil || result.Data.Revision != 2 || len(result.Data.Keys) != 2 {
		t.Fatal(result)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), "synthetic") {
		t.Fatal("secret leaked")
	}
	conflict := b.ProviderAction(Action{ProviderID: "p", ExpectedRevision: 1, Action: "strategy", Strategy: "priority"})
	if conflict.Problem == nil || conflict.Problem.Code != "conflict" {
		t.Fatal(conflict)
	}
}
