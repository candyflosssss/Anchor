package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

type firstOutputWriter func([]byte) (int, error)

func (w firstOutputWriter) Write(p []byte) (int, error) { return w(p) }

func TestFirstOutputRecognizesResponsesContentBeforeNativeDelivery(t *testing.T) {
	for _, tc := range []struct {
		name, event string
		generated   bool
	}{
		{"reasoning_delta", `{"type":"response.reasoning_summary_text.delta","delta":"thought"}`, true},
		{"reasoning_item", `{"type":"response.output_item.added","item":{"type":"reasoning","summary":[{"type":"summary_text","text":"thought"}]}}`, true},
		{"reasoning_item_done", `{"type":"response.output_item.done","item":{"type":"reasoning","content":[{"type":"reasoning_text","text":"thought"}]}}`, true},
		{"reasoning_encrypted", `{"type":"response.output_item.done","item":{"type":"reasoning","encrypted_content":"synthetic-ciphertext"}}`, true},
		{"reasoning_part", `{"type":"response.reasoning_summary_part.added","part":{"type":"summary_text","text":"thought"}}`, true},
		{"reasoning_part_done", `{"type":"response.reasoning_summary_part.done","part":{"type":"summary_text","text":"thought"}}`, true},
		{"reasoning_done", `{"type":"response.reasoning_summary_text.done","text":"thought"}`, true},
		{"message_item", `{"type":"response.output_item.added","item":{"type":"message","content":[{"type":"output_text","text":"reply"}]}}`, true},
		{"text_part", `{"type":"response.content_part.added","part":{"type":"output_text","text":"reply"}}`, true},
		{"text_done", `{"type":"response.output_text.done","text":"reply"}`, true},
		{"tool_item", `{"type":"response.output_item.done","item":{"type":"function_call","arguments":"{}"}}`, true},
		{"tool_done", `{"type":"response.function_call_arguments.done","arguments":"{}"}`, true},
		{"custom_item", `{"type":"response.output_item.done","item":{"type":"custom_tool_call","input":"patch"}}`, true},
		{"custom_done", `{"type":"response.custom_tool_call_input.done","input":"patch"}`, true},
		{"refusal", `{"type":"response.refusal.delta","delta":"cannot comply"}`, true},
		{"terminal_snapshot", `{"type":"response.completed","response":{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"reply"}]}]}}`, true},
		{"empty_reasoning", `{"type":"response.output_item.added","item":{"type":"reasoning","summary":[],"encrypted_content":""}}`, false},
		{"empty_part", `{"type":"response.reasoning_summary_part.added","part":{"type":"summary_text","text":""}}`, false},
		{"empty_delta", `{"type":"response.reasoning_summary_text.delta","delta":""}`, false},
		{"empty_done", `{"type":"response.reasoning_summary_text.done","text":""}`, false},
		{"tool_name_only", `{"type":"response.output_item.added","item":{"type":"function_call","name":"lookup","arguments":""}}`, false},
		{"unknown_part", `{"type":"response.content_part.added","part":{"type":"unknown","text":"not recognized"}}`, false},
	} {
		for _, eventOnly := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/event_only=%t", tc.name, eventOnly), func(t *testing.T) {
				s, err := NewSession([]byte(`{"model":"public","input":"hi","stream":true}`), "responses")
				if err != nil {
					t.Fatal(err)
				}
				a, err := s.Prepare(AttemptOptions{Protocol: "responses", Model: "actual", PublicModel: "public"})
				if err != nil {
					t.Fatal(err)
				}
				frame := "data: " + tc.event + "\n\n"
				if eventOnly {
					m, _ := Decode([]byte(tc.event))
					typ := str(m["type"])
					delete(m, "type")
					payload, _ := json.Marshal(m)
					frame = "event: " + typ + "\ndata: " + string(payload) + "\n\n"
				}
				raw := frame + frame + "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n"
				if tc.name == "terminal_snapshot" {
					raw = frame
				}
				count := 0
				var wire bytes.Buffer
				writer := firstOutputWriter(func(p []byte) (int, error) {
					if tc.generated && count != 1 {
						t.Fatal("first output notification must precede downstream delivery")
					}
					return wire.Write(p)
				})
				result, err := a.TranslateStream(context.Background(), writer, strings.NewReader(raw), StreamOptions{OnFirstOutput: func() { count++ }})
				if err != nil || (count == 1) != tc.generated || count > 1 || wire.String() != raw {
					t.Fatalf("notification=%d generated=%t native_preserved=%t err=%v", count, tc.generated, wire.String() == raw, err)
				}
				if strings.Contains(tc.name, "reasoning") && !result.Observation.FirstOutput.IsZero() {
					t.Fatal("reasoning must not change existing first usable output metric")
				}
			})
		}
	}
}

func TestFirstOutputFromEventNamesAcrossStreamConversions(t *testing.T) {
	streams := map[string]string{
		"responses": "event: response.output_item.added\ndata: {\"output_index\":0,\"item\":{\"type\":\"message\"}}\n\nevent: response.output_text.delta\ndata: {\"output_index\":0,\"delta\":\"reply\"}\n\nevent: response.completed\ndata: {\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n",
		"anthropic": "event: message_start\ndata: {\"message\":{\"id\":\"r\",\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: content_block_start\ndata: {\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"reply\"}}\n\nevent: message_delta\ndata: {\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\nevent: message_stop\ndata: {}\n\n",
	}
	for from, raw := range streams {
		for _, to := range []string{"responses", "chat-completions", "anthropic"} {
			t.Run(from+"_to_"+to, func(t *testing.T) {
				count := 0
				wire := httptest.NewRecorder()
				_, err := StreamObservedOptions(wire, strings.NewReader(raw), from, to, "public", nil, nil, StreamOptions{OnFirstOutput: func() { count++ }})
				if err != nil || count != 1 || !strings.Contains(wire.Body.String(), "reply") {
					t.Fatalf("converted output lost: count=%d err=%v", count, err)
				}
				if from == to && wire.Body.String() != raw {
					t.Fatal("native event-only payload was changed")
				}
			})
		}
	}
}

func TestFirstOutputPrefersJSONTypeOverSSEEventName(t *testing.T) {
	raw := "event: response.reasoning_text.delta\ndata: {\"type\":\"response.created\",\"delta\":\"metadata\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n"
	count := 0
	_, err := StreamObservedOptions(httptest.NewRecorder(), strings.NewReader(raw), "responses", "responses", "public", nil, nil, StreamOptions{OnFirstOutput: func() { count++ }})
	if err != nil || count != 0 {
		t.Fatalf("event name overrode explicit JSON type: count=%d err=%v", count, err)
	}
}

func TestFirstOutputNotificationExcludesLifecycleAndIncludesReasoningAndTools(t *testing.T) {
	for _, tc := range []struct{ protocol, prefix, output, terminal string }{
		{"responses", `{"type":"response.created","response":{"id":"r"}}`, `{"type":"response.reasoning_summary_text.delta","delta":"thought"}`, `{"type":"response.completed","response":{"status":"completed","output":[]}}`},
		{"chat-completions", `{"choices":[{"index":0,"delta":{"role":"assistant"}}]}`, `{"choices":[{"index":0,"delta":{"reasoning_content":"thought"}}]}`, `[DONE]`},
		{"anthropic", `{"type":"ping"}`, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`, `{"type":"message_stop"}`},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			request := `{"model":"public","messages":[{"role":"user","content":"hi"}],"stream":true}`
			if tc.protocol == "responses" {
				request = `{"model":"public","input":"hi","stream":true}`
			}
			if tc.protocol == "anthropic" {
				request = `{"model":"public","messages":[{"role":"user","content":"hi"}],"stream":true,"max_tokens":10}`
			}
			s, err := NewSession([]byte(request), tc.protocol)
			if err != nil {
				t.Fatal(err)
			}
			a, err := s.Prepare(AttemptOptions{Protocol: tc.protocol, Model: "actual", PublicModel: "public"})
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			raw := "data: " + tc.prefix + "\n\n: heartbeat\n\ndata: " + tc.output + "\n\ndata: " + tc.output + "\n\ndata: " + tc.terminal + "\n\n"
			_, err = a.TranslateStream(context.Background(), io.Discard, strings.NewReader(raw), StreamOptions{OnFirstOutput: func() { count++ }})
			if err != nil || count != 1 {
				t.Fatalf("count=%d err=%v", count, err)
			}
			count = 0
			raw = "data: " + tc.prefix + "\n\n: heartbeat\n\ndata: " + tc.terminal + "\n\n"
			_, err = a.TranslateStream(context.Background(), io.Discard, strings.NewReader(raw), StreamOptions{OnFirstOutput: func() { count++ }})
			if err != nil || count != 0 {
				t.Fatalf("envelope treated as output count=%d err=%v", count, err)
			}
		})
	}
}
