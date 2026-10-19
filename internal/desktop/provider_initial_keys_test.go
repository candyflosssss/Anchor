package desktop

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestProviderInitialKeysSharedContractAndBridge(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/provider-create.json")
	if err != nil {
		t.Fatal(err)
	}
	var input SaveProvider
	if err = json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	b := NewBridge(testSession(t))
	requireReady(t, b)
	saved := b.SaveProvider(input)
	if saved.Problem != nil || saved.Data == nil || saved.Data.Revision != 1 || len(saved.Data.BaseURLs) != 2 {
		t.Fatal(saved)
	}
	workspace := b.ProviderWorkspace(WorkspaceQuery{ProviderID: saved.Data.ID})
	if workspace.Problem != nil || workspace.Data == nil || len(workspace.Data.Keys) != 3 {
		t.Fatal("incomplete workspace")
	}
	if workspace.Data.Keys[0].ID != "backup" || workspace.Data.Keys[0].Protocol != "anthropic" || workspace.Data.Keys[1].ID != "default" || workspace.Data.Keys[2].Protocol != "chat-completions" {
		t.Fatal("Key protocols changed")
	}
	rows := b.Providers()
	if rows.Problem != nil || len(rows.Data) != 1 || !rows.Data[0].HasSecret {
		t.Fatal(rows)
	}
	for _, result := range []any{saved, workspace, rows} {
		public, _ := json.Marshal(result)
		if strings.Contains(string(public), "synthetic-create-") || strings.Contains(string(public), "initial_keys") {
			t.Fatal("write-only credentials leaked")
		}
	}
	conflict := b.SaveProvider(input)
	if conflict.Problem == nil || conflict.Problem.Code != "conflict" {
		t.Fatal(conflict)
	}
	input.Provider = *saved.Data
	input.ExpectedRevision = saved.Data.Revision
	invalid := b.SaveProvider(input)
	if invalid.Problem == nil || invalid.Problem.Code != "invalid" {
		t.Fatal(invalid)
	}
	input.InitialKeys = nil
	input.Secret = nil
	input.Provider.BaseURLs = []string{"https://edited.example.invalid/v1", "https://192.0.2.20:8443/v1"}
	edited := b.SaveProvider(input)
	if edited.Problem != nil || edited.Data == nil || edited.Data.Revision != 2 || edited.Data.BaseURLs[0] != input.Provider.BaseURLs[0] {
		t.Fatal(edited)
	}
	workspace = b.ProviderWorkspace(WorkspaceQuery{ProviderID: saved.Data.ID})
	if workspace.Problem != nil || len(workspace.Data.Keys) != 3 || workspace.Data.Keys[0].Protocol != "anthropic" {
		t.Fatal("address update changed named Keys")
	}
}
