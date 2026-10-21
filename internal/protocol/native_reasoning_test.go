package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

func TestNativeReasoningSanitization(t *testing.T) {
	for _, value := range []string{`null`, `false`, `42`, `[]`, `{}`, `""`} {
		t.Run(value, func(t *testing.T) {
			raw := fmt.Sprintf(`{"store":true,"input":[{"type":"reasoning","id":"rs_bad","encrypted_content":%s,"summary":[{"type":"summary_text","text":"keep summary"}],"extension":9007199254740993},{"type":"reasoning","id":"rs_good","encrypted_content":"opaque-future-format","summary":[]},{"type":"reasoning","id":"rs_missing","summary":[]},{"type":"compaction","encrypted_content":null},{"type":"function_call","call_id":"c","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"c","output":"result"},{"role":"user","content":"continue"}]}`, value)
			s := newTestSession(t, raw, "responses")
			baseline := append([]byte(nil), s.PolicyFields()["input"]...)
			for attempt := 0; attempt < 2; attempt++ {
				a := prepareTestAttempt(t, s, "responses")
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(a.RequestBody(), &fields); err != nil {
					t.Fatal(err)
				}
				var got, original []map[string]json.RawMessage
				json.Unmarshal(fields["input"], &got)
				json.Unmarshal(baseline, &original)
				if len(got) != len(original) || string(fields["store"]) != "false" {
					t.Fatal("dispatch shape changed", string(a.RequestBody()))
				}
				delete(original[0], "encrypted_content")
				delete(original[0], "id")
				want, _ := json.Marshal(original)
				actual, _ := json.Marshal(got)
				if !bytes.Equal(actual, want) {
					t.Fatalf("unexpected sanitization: %s != %s", actual, want)
				}
			}
			if !bytes.Equal(s.PolicyFields()["input"], baseline) || string(s.native) != raw {
				t.Fatal("client baseline mutated")
			}
		})
	}
}

func TestNativeReasoningSanitizationPreservesOpaqueAndOtherInput(t *testing.T) {
	for _, input := range []string{
		`"hello"`, `null`, `[]`, `[null,1,"unknown"]`,
		`[{"type":"reasoning","id":"keep","encrypted_content":"opaque","summary":[]}]`,
		`[{"type":"reasoning","encrypted_content":"  opaque  "}]`,
		`[{"type":"reasoning","id":"reference","summary":[]}]`,
		`[{"type":"agent_message","content":[{"type":"encrypted_content","encrypted_content":null}]}]`,
	} {
		if got := sanitizeNativeReasoningInput(json.RawMessage(input)); string(got) != input {
			t.Fatalf("input changed: %s => %s", input, got)
		}
	}
}
