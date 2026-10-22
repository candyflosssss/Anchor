package desktop

import (
	"encoding/json"
	"os"
	"reflect"
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
	p := b.SaveProvider(SaveProvider{Provider: Provider{ID: "p", BaseURLs: []string{"http://127.0.0.1:1/v1"}, Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 1}, Secret: &secret})
	if p.Problem != nil || p.Data == nil {
		t.Fatal(p)
	}
	r := b.ProviderWorkspace(WorkspaceQuery{ProviderID: "p"})
	if r.Problem != nil || r.Data == nil || len(r.Data.Keys) != 1 {
		t.Fatal(r)
	}
	result := b.ProviderAction(Action{ProviderID: "p", ExpectedRevision: 1, Action: "key-put", Target: "备用密钥", Secret: "synthetic-second", Enabled: true})
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

func TestSharedMappingBridgeReadWrite(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	for _, id := range []string{"a", "b"} {
		out := b.SaveProvider(SaveProvider{Provider: Provider{ID: id, BaseURLs: []string{"http://127.0.0.1:1/v1"}, Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 1}})
		if out.Problem != nil {
			t.Fatal(out)
		}
		outAction := b.ProviderAction(Action{ProviderID: id, ExpectedRevision: 1, Action: "model-put", Target: "shared", Model: "up-" + id, Multiplier: 2, Strategy: "round-robin"})
		if outAction.Problem != nil || outAction.Data == nil {
			t.Fatal(outAction)
		}
	}
	out := b.ProviderWorkspace(WorkspaceQuery{ProviderID: "a"})
	if out.Problem != nil || len(out.Data.Models) != 1 || out.Data.Models[0].CandidateCount != 2 || out.Data.Models[0].Multiplier != 2 || out.Data.Models[0].Strategy != "round-robin" {
		t.Fatal(out)
	}
	removed := b.ProviderAction(Action{ProviderID: "a", ExpectedRevision: 1, Action: "model-delete", Target: "shared", ModelRevision: 2, Approved: true})
	if removed.Problem != nil || len(removed.Data.Models) != 0 {
		t.Fatal(removed)
	}
	other := b.ProviderWorkspace(WorkspaceQuery{ProviderID: "b"})
	if other.Problem != nil || len(other.Data.Models) != 1 || other.Data.Models[0].CandidateCount != 1 || other.Data.Models[0].Revision != 3 {
		t.Fatal(other)
	}
}

func TestSharedModelMappingFixture(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/provider-model-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	var m ModelMapping
	if err = json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Multiplier != 1 || m.Strategy != "round-robin" || m.CandidateCount != 2 {
		t.Fatal(m)
	}
	encoded, _ := json.Marshal(m)
	var actual, expected any
	json.Unmarshal(encoded, &actual)
	json.Unmarshal(raw, &expected)
	a, _ := json.Marshal(actual)
	b, _ := json.Marshal(expected)
	if string(a) != string(b) {
		t.Fatal("mapping fixture drift")
	}
}

func TestRoutingBridgePreservesSourceKeysAndRejectsStaleWrites(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	secret := "synthetic-source"
	for _, id := range []string{"a", "b"} {
		out := b.SaveProvider(SaveProvider{Provider: Provider{ID: id, BaseURLs: []string{"http://127.0.0.1:1/v1"}, Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 1}, Secret: &secret})
		if out.Problem != nil {
			t.Fatal(out)
		}
	}
	for _, id := range []string{"a", "b"} {
		out := b.ProviderAction(Action{ProviderID: id, ExpectedRevision: 1, Action: "model-put", Target: "public", Model: "up-" + id, Multiplier: 2, Tools: true, Streaming: true, KeyIDs: []string{"default"}})
		if out.Problem != nil {
			t.Fatal(out)
		}
	}
	in := ModelRouting{ModelID: "public", ExpectedRevision: 2, Strategy: "cache-priority", Order: []RouteCandidate{{ProviderID: "b", Model: "up-b"}, {ProviderID: "a", Model: "up-a"}}}
	out := b.ConfigureModelRouting(in)
	if out.Problem != nil || out.Data == nil || out.Data.Revision != 3 {
		t.Fatal(out)
	}
	w := b.ProviderWorkspace(WorkspaceQuery{ProviderID: "b"})
	if w.Problem != nil || w.Data.Models[0].CandidateIndex != 0 || w.Data.Models[0].Strategy != "cache-priority" || len(w.Data.Models[0].KeyIDs) != 1 {
		t.Fatal(w)
	}
	out = b.ConfigureModelRouting(in)
	if out.Problem == nil || out.Problem.Code != "conflict" {
		t.Fatal(out)
	}
	raw, _ := json.Marshal(w)
	if strings.Contains(string(raw), secret) {
		t.Fatal("secret returned")
	}
}

func TestSharedRoutingContract(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/model-routing.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Input    ModelRouting      `json:"input"`
		Success  ModelRoutingState `json:"success"`
		KeyOrder struct {
			Input   ModelRouting      `json:"input"`
			Success ModelRoutingState `json:"success"`
		} `json:"key_order"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Success.Revision != fixture.Input.ExpectedRevision+1 || fixture.Success.Strategy != fixture.Input.Strategy || len(fixture.Success.Order) != 2 {
		t.Fatal(fixture)
	}
	if len(fixture.KeyOrder.Input.SourceOrder) != 3 || !reflect.DeepEqual(fixture.KeyOrder.Input.SourceOrder, fixture.KeyOrder.Success.SourceOrder) {
		t.Fatal("shared Key order fixture", fixture.KeyOrder)
	}
}
