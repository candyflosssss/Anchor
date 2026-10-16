package catalog

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestChatSharedFixtureAndValidation(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/chat.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Request  ChatRequest   `json:"request"`
		Progress ProbeProgress `json:"progress"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Request.Validate(); err != nil {
		t.Fatal(err)
	}
	if fixture.Progress.Result.Failure != "" || fixture.Progress.KeyID != fixture.Request.KeyID {
		t.Fatal("fixture drift")
	}
	for _, mutate := range []func(*ChatRequest){
		func(in *ChatRequest) { in.Approved = false },
		func(in *ChatRequest) { in.ExpectedRevision = 0 },
		func(in *ChatRequest) { in.KeyID = "" },
		func(in *ChatRequest) { in.Model = "m\nheader" },
		func(in *ChatRequest) { in.Model = " m" },
		func(in *ChatRequest) { in.ReasoningEffort = "extreme" },
		func(in *ChatRequest) { in.ReasoningEffort = " high" },
		func(in *ChatRequest) { in.Messages = nil },
		func(in *ChatRequest) { in.Messages = []ChatMessage{{"system", "not allowed"}} },
		func(in *ChatRequest) { in.Messages = []ChatMessage{{"user", "one"}, {"user", "two"}} },
		func(in *ChatRequest) { in.Messages = []ChatMessage{{"user", " "}} },
		func(in *ChatRequest) { in.Messages = []ChatMessage{{"user", strings.Repeat("中", ChatMaxBytes/3+1)}} },
		func(in *ChatRequest) { in.RunID = "unexpected" },
		func(in *ChatRequest) { in.Action = "unknown" },
	} {
		in := fixture.Request
		mutate(&in)
		if in.Validate() == nil {
			t.Fatalf("accepted invalid request: %+v", in)
		}
	}
	in := fixture.Request
	in.Action = "cancel"
	in.RunID = "synthetic-run"
	in.Messages = nil
	if in.Validate() != nil {
		t.Fatal("cannot cancel run without message history")
	}
}

func TestChatReasoningLevels(t *testing.T) {
	in := ChatRequest{ProviderID: "p", ExpectedRevision: 1, KeyID: "default", Model: "m", Approved: true, Action: "start", Messages: []ChatMessage{{"user", "hi"}}}
	for _, effort := range []string{"", "none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		in.ReasoningEffort = effort
		if err := in.Validate(); err != nil {
			t.Fatalf("effort %q: %v", effort, err)
		}
		raw, _ := json.Marshal(in)
		if effort == "" && strings.Contains(string(raw), "reasoning_effort") {
			t.Fatal("default effort must be omitted")
		}
	}
}
