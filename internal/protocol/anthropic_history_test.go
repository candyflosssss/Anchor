package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnthropicForeignReasoningHistoryProjection(t *testing.T) {
	for _, fields := range []string{
		`"summary":[],"encrypted_content":"PRIVATE_CIPHER"`,
		`"summary":[{"type":"summary_text","text":"PRIVATE_THOUGHT"}],"encrypted_content":"PRIVATE_CIPHER"`,
		`"content":[{"type":"reasoning_text","text":"PRIVATE_THOUGHT"}]`,
	} {
		raw := `{"model":"public","input":[{"role":"user","content":"question"},{"type":"reasoning","id":"rs_keep","status":"completed",` + fields + `},{"type":"message","role":"assistant","phase":"commentary","content":[{"type":"output_text","text":"checking"}]},{"type":"function_call","id":"fc_keep","name":"lookup","call_id":"call_keep","arguments":"{\"x\":1}"},{"type":"function_call_output","call_id":"call_keep","output":"TOOL_RESULT"},{"type":"message","role":"assistant","phase":"final_answer","content":[{"type":"output_text","text":"ANSWER"}]},{"role":"user","content":"continue"}],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`
		s := newTestSession(t, raw, "responses")
		a := prepareTestAttempt(t, s, "anthropic")
		body := string(a.RequestBody())
		for _, bad := range []string{"PRIVATE_", "rs_keep", `"reasoning"`, `"phase"`, `"signature"`} {
			if strings.Contains(body, bad) {
				t.Fatal("foreign reasoning or phase forwarded")
			}
		}
		for _, keep := range []string{"question", "checking", "call_keep", "TOOL_RESULT", "ANSWER", "continue", `"x":1`} {
			if !strings.Contains(body, keep) {
				t.Fatal("conversation or tool lost", keep)
			}
		}
		if a.Diagnostics().Compatibility != Degraded || !hasDegradation(a.Diagnostics(), "reasoning_history_omitted") || !hasDegradation(a.Diagnostics(), "message_phase_omitted") {
			t.Fatal(a.Diagnostics())
		}
		native := prepareTestAttempt(t, s, "responses")
		original, _ := Decode([]byte(raw))
		preserved, _ := Decode(native.RequestBody())
		before, _ := json.Marshal(original["input"])
		after, _ := json.Marshal(preserved["input"])
		if string(before) != string(after) {
			t.Fatal("changed original reasoning or phase")
		}
	}
}

func TestAnthropicPlaintextReasoningFromChat(t *testing.T) {
	s := newTestSession(t, `{"model":"public","messages":[{"role":"user","content":"question"},{"role":"assistant","content":"answer","reasoning_content":"PRIVATE_THOUGHT"},{"role":"user","content":"continue"}]}`, "chat-completions")
	a := prepareTestAttempt(t, s, "anthropic")
	if strings.Contains(string(a.RequestBody()), "PRIVATE_THOUGHT") || !hasDegradation(a.Diagnostics(), "reasoning_history_omitted") {
		t.Fatal(a.Diagnostics())
	}
	if !strings.Contains(string(prepareTestAttempt(t, s, "chat-completions").RequestBody()), "PRIVATE_THOUGHT") {
		t.Fatal("native reasoning lost")
	}
}

func TestAnthropicHistoryProjectionRejectsMalformedAndUnknown(t *testing.T) {
	for _, item := range []string{
		`{"type":"reasoning","summary":"PRIVATE_VALUE","encrypted_content":"PRIVATE_CIPHER"}`,
		`{"type":"reasoning","summary":[{"type":"summary_text","text":12}],"encrypted_content":"PRIVATE_CIPHER"}`,
		`{"type":"reasoning","summary":[],"private-field":"PRIVATE_VALUE","encrypted_content":"PRIVATE_CIPHER"}`,
		`{"type":"reasoning","summary":[],"encrypted_content":12}`,
		`{"type":"reasoning","summary":[],"encrypted_content":{}}`,
		`{"type":"message","role":"assistant","phase":"PRIVATE_VALUE","content":"answer"}`,
		`{"type":"message","role":"assistant","phase":12,"content":"answer"}`,
		`{"type":"message","role":"user","phase":"commentary","content":"question"}`,
		`{"type":"compaction","encrypted_content":"PRIVATE_CIPHER"}`,
		`{"type":"function_call_output","call_id":"c","output":[{"type":"encrypted_content","data":"PRIVATE_CIPHER"}]}`,
	} {
		s := newTestSession(t, `{"input":[{"role":"user","content":"hello"},`+item+`]}`, "responses")
		_, err := s.Prepare(AttemptOptions{Protocol: "anthropic"})
		if err == nil || strings.Contains(err.Error(), "PRIVATE") || strings.Contains(err.Error(), "private-field") {
			t.Fatal("unsafe acceptance or diagnostics", err)
		}
		if _, err = s.Prepare(AttemptOptions{Protocol: "responses"}); err != nil {
			t.Fatal("native path changed", err)
		}
	}
}
