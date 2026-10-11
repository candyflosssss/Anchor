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
