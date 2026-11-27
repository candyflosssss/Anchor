package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

// Codex goal mode stores a function_call_output with namespace/name and no
// call_id; Messages and Chat reject unpaired tool results, so it becomes user
// content with an explicit degradation instead of failing the whole session.
func TestOrphanToolOutputBecomesUserContent(t *testing.T) {
	inputs := map[string]string{
		"goal_without_call_id": `{"type":"function_call_output","id":"fco_1","namespace":"agent","name":"goal_objective","output":"GOAL_TEXT"}`,
		"unknown_call_id":      `{"type":"function_call_output","call_id":"never_called","output":[{"type":"input_text","text":"GOAL_TEXT"}]}`,
	}
	for name, orphan := range inputs {
		for _, to := range []string{"anthropic", "chat-completions"} {
			t.Run(name+"/"+to, func(t *testing.T) {
				raw := `{"model":"public","input":[{"role":"user","content":"QUESTION"},{"type":"function_call","call_id":"c1","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"c1","output":"PAIRED"},` + orphan + `,{"role":"user","content":"NEXT"}],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`
				s := newTestSession(t, raw, "responses")
				a := prepareTestAttempt(t, s, to)
				body := string(a.RequestBody())
				if !strings.Contains(body, "GOAL_TEXT") || !strings.Contains(body, "PAIRED") || strings.Contains(body, "goal_objective") || strings.Contains(body, "never_called") || strings.Contains(body, `"namespace"`) {
					t.Fatal(body)
				}
				if to == "anthropic" {
					if strings.Count(body, `"tool_result"`) != 1 || !strings.Contains(body, `"tool_use_id":"c1"`) {
						t.Fatal(body)
					}
				} else if strings.Count(body, `"role":"tool"`) != 1 || !strings.Contains(body, `"tool_call_id":"c1"`) {
					t.Fatal(body)
				}
				if a.Diagnostics().Compatibility != Degraded || !hasDegradation(a.Diagnostics(), OrphanToolOutputText) {
					t.Fatal(a.Diagnostics())
				}
				native := prepareTestAttempt(t, s, "responses")
				if !strings.Contains(string(native.RequestBody()), "GOAL_TEXT") || hasDegradation(native.Diagnostics(), OrphanToolOutputText) {
					t.Fatal(string(native.RequestBody()))
				}
			})
		}
	}
}

func TestPairedToolOutputStaysFull(t *testing.T) {
	raw := `{"model":"public","input":[{"role":"user","content":"q"},{"type":"function_call","call_id":"c1","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"c1","output":"r"}],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`
	for _, to := range []string{"anthropic", "chat-completions"} {
		a := prepareTestAttempt(t, newTestSession(t, raw, "responses"), to)
		if hasDegradation(a.Diagnostics(), OrphanToolOutputText) || a.Diagnostics().Compatibility != Full {
			t.Fatal(to, a.Diagnostics())
		}
	}
}

func TestOrphanToolOutputAnthropicImageAndEmpty(t *testing.T) {
	raw := `{"model":"public","input":[{"role":"user","content":"q"},{"type":"function_call_output","output":[{"type":"input_text","text":"see"},{"type":"input_image","image_url":"https://example.com/a.png"}]},{"type":"function_call_output","output":""}]}`
	a := prepareTestAttempt(t, newTestSession(t, raw, "responses"), "anthropic")
	wire, _ := Decode(a.RequestBody())
	messages := arr(wire["messages"])
	body, _ := json.Marshal(messages)
	if len(messages) != 1 || !strings.Contains(string(body), `"url":"https://example.com/a.png"`) || strings.Contains(string(body), "tool_result") || strings.Contains(string(body), `"text":""`) {
		t.Fatal(string(body))
	}
}

func TestOrphanToolOutputRejectsUnknownShapes(t *testing.T) {
	for _, orphan := range []string{
		`{"type":"function_call_output","name":"goal_objective","output":"x","extra":1}`,
		`{"type":"function_call_output","namespace":{"x":1},"output":"x"}`,
		`{"type":"function_call_output","output":{"type":"unknown"}}`,
		`{"type":"function_call_output","output":[{"type":"input_file","file_id":"f"}]}`,
	} {
		for _, to := range []string{"anthropic", "chat-completions"} {
			s := newTestSession(t, `{"model":"public","input":[{"role":"user","content":"q"},`+orphan+`]}`, "responses")
			if _, err := s.Prepare(AttemptOptions{Protocol: to, Model: "selected", PublicModel: "public"}); err == nil {
				t.Fatal(to, orphan)
			}
		}
	}
	// A paired output keeps the strict paired field set.
	s := newTestSession(t, `{"model":"public","input":[{"type":"function_call","call_id":"c","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"c","name":"lookup","output":"x"}],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`, "responses")
	if _, err := s.Prepare(AttemptOptions{Protocol: "anthropic", Model: "selected", PublicModel: "public"}); err == nil {
		t.Fatal("paired output with extra identity fields must stay rejected")
	}
}
