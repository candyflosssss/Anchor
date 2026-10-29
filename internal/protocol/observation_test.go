package protocol

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogUsagePresenceAndMalformedValues(t *testing.T) {
	for _, tc := range []struct {
		body                    string
		known, cache, reasoning bool
	}{
		{`{"usage":{"input_tokens":0,"output_tokens":0,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`, true, true, true},
		{`{"usage":{"input_tokens":1,"output_tokens":2}}`, true, false, false},
		{`{"usage":{"input_tokens":1,"output_tokens":2,"output_tokens_details":{"reasoning_tokens":3}}}`, false, false, false},
		{`{"usage":{"input_tokens":1.5,"output_tokens":2}}`, false, false, false},
		{`{"usage":{"input_tokens":"1","output_tokens":2}}`, false, false, false},
		{`{}`, false, false, false},
	} {
		m, err := Decode([]byte(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		u := ReadUsage(m, "responses")
		if u.Known != tc.known || u.CacheReadKnown != tc.cache || u.ReasoningKnown != tc.reasoning {
			t.Fatalf("%s: %+v", tc.body, u)
		}
	}
}
func TestStructuralStreamObservations(t *testing.T) {
	for _, tc := range []struct {
		protocol, stream         string
		text, nontext, reasoning bool
		status                   string
	}{
		{"responses", "data: {\"type\":\"response.reasoning_text.delta\",\"delta\":\"private\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", false, false, true, "completed"},
		{"responses", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"private\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", true, false, false, "completed"},
		{"chat-completions", "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n", false, true, false, "completed"},
	} {
		t.Run(tc.protocol+tc.status, func(t *testing.T) {
			w := httptest.NewRecorder()
			o := Observation{}
			_, err := StreamObserved(w, strings.NewReader(tc.stream), tc.protocol, tc.protocol, "model", &o)
			if err != nil {
				t.Fatal(err)
			}
			if !o.FirstText.IsZero() != tc.text || o.NonText != tc.nontext || o.Reasoning != tc.reasoning || o.Status != tc.status || o.Terminal.IsZero() {
				t.Fatalf("%+v", o)
			}
		})
	}
}

func TestFirstOutputObservesTextAndToolArgumentsOnly(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, event string
		output, text          bool
	}{
		{"responses_text", "responses", `{"type":"response.output_text.delta","delta":"text"}`, true, true},
		{"responses_tool", "responses", `{"type":"response.function_call_arguments.delta","delta":"{"}`, true, false},
		{"responses_custom_tool", "responses", `{"type":"response.custom_tool_call_input.delta","delta":"patch"}`, true, false},
		{"responses_empty_tool", "responses", `{"type":"response.function_call_arguments.delta","delta":""}`, false, false},
		{"responses_empty_text", "responses", `{"type":"response.output_text.delta","delta":""}`, false, false},
		{"responses_tool_envelope", "responses", `{"type":"response.output_item.added","item":{"type":"function_call","name":"exec"}}`, false, false},
		{"responses_created", "responses", `{"type":"response.created"}`, false, false},
		{"responses_reasoning", "responses", `{"type":"response.reasoning_text.delta","delta":"thinking"}`, false, false},
		{"chat_text", "chat-completions", `{"choices":[{"delta":{"content":"text"}}]}`, true, true},
		{"chat_tool", "chat-completions", `{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{"}}]}}]}`, true, false},
		{"chat_tool_envelope", "chat-completions", `{"choices":[{"delta":{"role":"assistant","tool_calls":[{"id":"call","function":{"name":"exec","arguments":""}}]}}]}`, false, false},
		{"chat_reasoning", "chat-completions", `{"choices":[{"delta":{"reasoning_content":"thinking"}}]}`, false, false},
		{"anthropic_text_start", "anthropic", `{"type":"content_block_start","content_block":{"type":"text","text":"text"}}`, true, true},
		{"anthropic_text", "anthropic", `{"type":"content_block_delta","delta":{"type":"text_delta","text":"text"}}`, true, true},
		{"anthropic_tool", "anthropic", `{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{"}}`, true, false},
		{"anthropic_tool_envelope", "anthropic", `{"type":"content_block_start","content_block":{"type":"tool_use","input":{}}}`, false, false},
		{"anthropic_empty_tool", "anthropic", `{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":""}}`, false, false},
		{"anthropic_thinking", "anthropic", `{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"thinking"}}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Decode([]byte(tc.event))
			if err != nil {
				t.Fatal(err)
			}
			o := Observation{}
			o.event(m, tc.protocol)
			if !o.FirstOutput.IsZero() != tc.output || !o.FirstText.IsZero() != tc.text {
				t.Fatalf("unexpected first output/text: %+v", o)
			}
			if tc.output {
				first := o.FirstOutput
				o.event(m, tc.protocol)
				if !o.FirstOutput.Equal(first) {
					t.Fatal("later delta replaced first output")
				}
			}
		})
	}
}

func TestToolOutputAndFirstTextUseSeparateClocks(t *testing.T) {
	o := Observation{}
	o.event(Object{"type": "response.function_call_arguments.delta", "delta": "{"}, "responses")
	first := o.FirstOutput
	o.event(Object{"type": "response.output_text.delta", "delta": "text"}, "responses")
	if first.IsZero() || !o.FirstOutput.Equal(first) || o.FirstText.IsZero() || o.FirstText.Before(first) {
		t.Fatalf("text must not overwrite first tool output: %+v", o)
	}
}

func TestToolFirstOutputAcrossStreamConversions(t *testing.T) {
	streams := map[string]string{
		"responses":        "data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"function_call\",\"call_id\":\"call\",\"name\":\"exec\"}}\n\ndata: {\"type\":\"response.function_call_arguments.delta\",\"output_index\":0,\"delta\":\"{}\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n",
		"chat-completions": "data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call\",\"function\":{\"name\":\"exec\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n",
		"anthropic":        "data: {\"type\":\"message_start\",\"message\":{\"id\":\"r\",\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"call\",\"name\":\"exec\",\"input\":{}}}\n\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{}\"}}\n\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n",
	}
	for from, stream := range streams {
		for to := range streams {
			t.Run(from+"_to_"+to, func(t *testing.T) {
				o := Observation{}
				_, err := StreamObserved(httptest.NewRecorder(), strings.NewReader(stream), from, to, "model", &o)
				if err != nil || o.FirstOutput.IsZero() || !o.FirstText.IsZero() || !o.NonText {
					t.Fatalf("tool output not observed independently of conversion: %+v %v", o, err)
				}
			})
		}
	}
}
