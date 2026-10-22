package protocol

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestResponsesToolIDsRepairDispatchOnly(t *testing.T) {
	raw := `{"input":[{"type":"custom_tool_call","id":"call_fixture","call_id":"call_fixture","name":"exec","namespace":"functions","input":"line\n雪😀","extension":9007199254740993},{"type":"custom_tool_call_output","id":"call_result","call_id":"call_fixture","output":[{"type":"input_text","text":"result"}]},{"type":"function_call","id":"call_function","call_id":"call_function","name":"lookup","arguments":"{}"},{"type":"function_call_output","id":"fco_keep","call_id":"call_function","output":"ok"},{"type":"reasoning","id":"item_opaque","encrypted_content":"opaque"},{"type":"message","id":"item_message","role":"user","content":"continue"}]}`
	s := newTestSession(t, raw, "responses")
	baseline := s.PolicyFields()["input"]
	var want []map[string]json.RawMessage
	if err := json.Unmarshal(baseline, &want); err != nil {
		t.Fatal(err)
	}
	want[0]["id"] = json.RawMessage(`"ctc_call_fixture"`)
	want[1]["id"] = json.RawMessage(`"ctco_call_result"`)
	want[2]["id"] = json.RawMessage(`"fc_call_function"`)
	wantJSON, _ := json.Marshal(want)
	for i := 0; i < 2; i++ {
		a := prepareTestAttempt(t, s, "responses")
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(a.RequestBody(), &fields); err != nil {
			t.Fatal(err)
		}
		var got []map[string]json.RawMessage
		json.Unmarshal(fields["input"], &got)
		gotJSON, _ := json.Marshal(got)
		if !bytes.Equal(gotJSON, wantJSON) {
			t.Fatalf("dispatch changed unrelated fields: %s != %s", gotJSON, wantJSON)
		}
	}
	if string(s.native) != raw || !bytes.Equal(s.PolicyFields()["input"], baseline) {
		t.Fatal("client history mutated")
	}
}

func TestResponsesToolIDsPreserveOtherInput(t *testing.T) {
	for _, input := range []string{
		`"hello"`, `null`, `[]`, `[null,1,"unknown"]`, `invalid`,
		`[{"type":"custom_tool_call","id":"ctc_valid","call_id":"call_keep"}]`,
		`[{"type":"custom_tool_call","id":"ctc-valid"}]`,
		`[{"type":"function_call","id":"fc_valid"},{"type":"custom_tool_call_output","id":"ctco_valid"}]`,
		`[{"type":"custom_tool_call","id":""},{"type":"custom_tool_call"},{"type":"custom_tool_call","id":null},{"type":"custom_tool_call","id":42}]`,
		`[{"type":"reasoning","id":"item_keep","encrypted_content":"opaque"},{"type":"unknown","id":"call_keep"}]`,
		`[{"role":"user","content":[{"type":"custom_tool_call","id":"call_nested"}]}]`,
	} {
		if got := sanitizeResponsesToolIDs(json.RawMessage(input)); string(got) != input {
			t.Fatalf("unrelated input changed: %s => %s", input, got)
		}
	}
}

func TestResponsesToolIDsDeterministicCollisionAndLengthHandling(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		items := []Object{
			{"type": "custom_tool_call", "id": "call_fixture", "call_id": "call_fixture"},
			{"type": "custom_tool_call", "id": "ctc_call_fixture", "call_id": "call_other"},
			{"type": "custom_tool_call", "id": strings.Repeat("雪", 80), "call_id": "unicode"},
			{"type": "custom_tool_call", "id": strings.Repeat("雪", 79) + "x", "call_id": "unicode_other"},
			{"type": "function_call", "id": "fc_" + strings.Repeat("x", 64), "call_id": "long"},
		}
		if reverse {
			for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
				items[i], items[j] = items[j], items[i]
			}
		}
		input, _ := json.Marshal(items)
		first := sanitizeResponsesToolIDs(input)
		if !bytes.Equal(first, sanitizeResponsesToolIDs(input)) || !bytes.Equal(first, sanitizeResponsesToolIDs(first)) {
			t.Fatal("repair is not deterministic and idempotent")
		}
		var got []Object
		json.Unmarshal(first, &got)
		seen := map[string]bool{}
		for _, item := range got {
			id := str(item["id"])
			if seen[id] || utf8.RuneCountInString(id) > 64 {
				t.Fatal("ID collision or oversized ID", id)
			}
			seen[id] = true
			prefix := "ctc"
			if item["type"] == "function_call" {
				prefix = "fc"
			}
			if !strings.HasPrefix(id, prefix) || item["call_id"] == "call_other" && id != "ctc_call_fixture" {
				t.Fatal("prefix invalid or existing valid ID changed", item)
			}
		}
	}
}

func TestResponsesToolIDsDoNotAffectChatCandidate(t *testing.T) {
	s := newTestSession(t, `{"input":[{"type":"custom_tool_call","id":"call_fixture","call_id":"call_fixture","name":"exec","input":"code"},{"type":"custom_tool_call_output","call_id":"call_fixture","output":"done"}],"tools":[{"type":"custom","name":"exec"}]}`, "responses")
	prepareTestAttempt(t, s, "responses")
	chat := prepareTestAttempt(t, s, "chat-completions")
	m, err := Decode(chat.RequestBody())
	if err != nil {
		t.Fatal(err)
	}
	messages := arr(m["messages"])
	call := obj(arr(obj(messages[0])["tool_calls"])[0])
	if call["id"] != "call_fixture" || obj(messages[1])["tool_call_id"] != "call_fixture" {
		t.Fatal("tool correlation changed for another candidate", messages)
	}
}

func TestResponsesToolIDsReserveUnknownItemIDs(t *testing.T) {
	input := json.RawMessage(`[{"type":42,"id":"ctc_call_fixture","extension":9007199254740993},{"type":"custom_tool_call","id":"call_fixture","call_id":"call_fixture"},{"type":"custom_tool_call","id":"call_fixture","call_id":"call_fixture"}]`)
	var original, got []map[string]json.RawMessage
	json.Unmarshal(input, &original)
	json.Unmarshal(sanitizeResponsesToolIDs(input), &got)
	before, _ := json.Marshal(original[0])
	after, _ := json.Marshal(got[0])
	if !bytes.Equal(before, after) || bytes.Equal(got[0]["id"], got[1]["id"]) || !bytes.Equal(got[1]["id"], got[2]["id"]) {
		t.Fatal("unknown ID collision, unrelated mutation or unstable duplicate mapping", got)
	}
}
