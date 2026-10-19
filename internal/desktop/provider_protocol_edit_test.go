package desktop

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProviderProtocolBridgeEdit(t *testing.T) {
	b := NewBridge(testSession(t))
	requireReady(t, b)
	secret := "synthetic-protocol-key"
	saved := b.SaveProvider(SaveProvider{Provider: Provider{ID: "protocol-edit", BaseURLs: []string{"http://127.0.0.1:1/v1"}, Protocol: "anthropic", Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 1}, Secret: &secret})
	if saved.Problem != nil || saved.Data == nil || saved.Data.Protocol != "anthropic" {
		t.Fatal(saved)
	}
	rows := b.Providers()
	if rows.Problem != nil || rows.Data == nil || len(rows.Data) != 1 || rows.Data[0].Protocol != "anthropic" {
		t.Fatal(rows)
	}
	named := b.ProviderAction(Action{ProviderID: saved.Data.ID, ExpectedRevision: saved.Data.Revision, Action: "key-put", Target: "named", Secret: "synthetic-named", Enabled: true, Protocol: "responses"})
	if named.Problem != nil || named.Data == nil {
		t.Fatal(named)
	}
	input := SaveProvider{Provider: *saved.Data, ExpectedRevision: named.Data.Revision}
	input.Provider.Revision = input.ExpectedRevision
	input.Provider.Protocol = "chat-completions"
	saved = b.SaveProvider(input)
	if saved.Problem != nil || saved.Data == nil || saved.Data.Protocol != "chat-completions" {
		t.Fatal(saved)
	}
	workspace := b.ProviderWorkspace(WorkspaceQuery{ProviderID: saved.Data.ID})
	if workspace.Problem != nil || workspace.Data == nil || len(workspace.Data.Keys) != 2 || workspace.Data.Keys[0].Protocol != "chat-completions" || workspace.Data.Keys[1].Protocol != "responses" {
		t.Fatal(workspace)
	}
	conflict := b.SaveProvider(input)
	if conflict.Problem == nil || conflict.Problem.Code != "conflict" {
		t.Fatal(conflict)
	}
	input.ExpectedRevision = saved.Data.Revision
	input.Provider.Revision = saved.Data.Revision
	input.Provider.Protocol = "invalid"
	invalid := b.SaveProvider(input)
	if invalid.Problem == nil || invalid.Problem.Code != "invalid" {
		t.Fatal(invalid)
	}
	input.Provider.Protocol = "" // Legacy/unchanged edits omit the optional field.
	input.Provider.TimeoutSeconds = 20
	preserved := b.SaveProvider(input)
	if preserved.Problem != nil || preserved.Data == nil || preserved.Data.Protocol != "chat-completions" {
		t.Fatal(preserved)
	}
	workspace = b.ProviderWorkspace(WorkspaceQuery{ProviderID: saved.Data.ID})
	if workspace.Problem != nil || workspace.Data == nil || workspace.Data.Keys[0].Protocol != "chat-completions" {
		t.Fatal(workspace)
	}
	raw, _ := json.Marshal(workspace)
	if strings.Contains(string(raw), "synthetic-") {
		t.Fatal("secret leaked")
	}
}
