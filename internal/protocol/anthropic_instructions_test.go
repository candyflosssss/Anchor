package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnthropicInstructionsWithinConversation(t *testing.T) {
	for _, client := range []string{"responses", "chat-completions"} {
		t.Run(client, func(t *testing.T) {
			messages := []any{
				Object{"role": "developer", "content": []any{Object{"type": "input_text", "text": "FIRST_DEVELOPER"}}},
				Object{"role": "user", "content": "FIRST_USER"},
				Object{"role": "assistant", "content": "ASSISTANT"},
				Object{"role": "system", "content": "LATER_SYSTEM"},
				Object{"role": "developer", "content": "LATER_DEVELOPER"},
				Object{"role": "user", "content": "CURRENT_USER"},
			}
			m := Object{"model": "public", "stream": true}
			if client == "responses" {
				m["input"], m["instructions"] = messages, "BASE_INSTRUCTIONS"
			} else {
				m["messages"] = messages
			}
			raw, _ := json.Marshal(m)
			s := newTestSession(t, string(raw), client)
			a := prepareTestAttempt(t, s, "anthropic")
			wire, _ := Decode(a.RequestBody())
			expected := "FIRST_DEVELOPER\nLATER_SYSTEM\nLATER_DEVELOPER"
			if client == "responses" {
				expected = "BASE_INSTRUCTIONS\n" + expected
			}
			if wire["system"] != expected {
				t.Fatal("instruction content or order lost", wire["system"])
			}
			body, _ := json.Marshal(wire["messages"])
			if strings.Contains(string(body), "DEVELOPER") || strings.Contains(string(body), "LATER_SYSTEM") || len(arr(wire["messages"])) != 3 {
				t.Fatal(string(body))
			}
			if a.Diagnostics().Compatibility != Degraded || !hasDegradation(a.Diagnostics(), "instruction_scope_hoisted") {
				t.Fatal(a.Diagnostics())
			}
			native := prepareTestAttempt(t, s, client)
			preserved, _ := Decode(native.RequestBody())
			key := "input"
			if client != "responses" {
				key = "messages"
			}
			original, _ := json.Marshal(messages)
			actual, _ := json.Marshal(preserved[key])
			if string(original) != string(actual) {
				t.Fatal("native candidate changed")
			}
		})
	}
}

func TestAnthropicPrefixInstructionsRemainFull(t *testing.T) {
	for _, raw := range []string{
		`{"input":[{"role":"developer","content":"initial"},{"role":"user","content":"question"}]}`,
		`{"input":[{"role":"user","content":"question"},{"role":"developer","content":""}]}`,
	} {
		a := prepareTestAttempt(t, newTestSession(t, raw, "responses"), "anthropic")
		if a.Diagnostics().Compatibility != Full || len(a.Diagnostics().Degradations) != 0 {
			t.Fatal(a.Diagnostics())
		}
	}
}
