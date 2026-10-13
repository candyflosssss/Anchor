package protocol

import (
	"errors"
	"testing"
)

func TestValidateProbeResponseReasoning(t *testing.T) {
	for _, raw := range []string{
		`{"id":"r","status":"completed","output":[{"type":"reasoning","id":"rs","summary":[]},{"type":"message","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":3,"output_tokens":16}}`,
		`{"id":"r","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"reasoning","id":"rs","summary":[]}],"usage":{"input_tokens":3,"output_tokens":16}}`,
	} {
		u, err := ValidateProbeResponse([]byte(raw), "responses")
		if err != nil || !u.Known || u.Input != 3 || u.Output != 16 {
			t.Fatalf("usage=%+v err=%v", u, err)
		}
		// Converting reasoning to other protocols must still fail explicitly.
		if _, err := ParseResponse([]byte(raw), "responses"); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("conversion silently dropped reasoning: %v", err)
		}
	}
}

func TestValidateProbeResponseStillRejectsInvalidOutput(t *testing.T) {
	for _, raw := range []string{
		`not json`,
		`{"output":[{"type":"reasoning","summary":[]}]}`,
		`{"id":"r","status":"failed","output":[{"type":"reasoning","summary":[]}]}`,
		`{"id":"r","error":{"message":"synthetic error"},"output":[{"type":"reasoning","summary":[]}]}`,
		`{"id":"r","output":[{"type":"reasoning","summary":[]},{"type":"unknown"}]}`,
		`{"id":"r","output":[{"type":"reasoning","summary":[]},{"type":"function_call","call_id":"c","name":"f","arguments":"invalid"}]}`,
	} {
		if _, err := ValidateProbeResponse([]byte(raw), "responses"); err == nil {
			t.Fatalf("invalid response accepted: %s", raw)
		}
	}
}

func TestProbeTextMetadataDoesNotRequireConversion(t *testing.T) {
	for _, metadata := range []string{
		`"logprobs":[]`,
		`"logprobs":[{"token":"OK","logprob":-0.1,"top_logprobs":[]}]`,
		`"annotations":[{"type":"url_citation","url":"https://example.invalid","title":"private metadata","start_index":0,"end_index":2}]`,
		`"logprobs":null,"annotations":null`,
	} {
		t.Run(metadata, func(t *testing.T) {
			raw := []byte(`{"id":"r","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"OK",` + metadata + `}]}],"usage":{"input_tokens":3,"output_tokens":16}}`)
			// The previous probe reused this conversion parser, which rejects
			// metadata it cannot preserve in another protocol.
			_, conversionErr := ParseResponse(raw, "responses")
			if metadata == `"logprobs":null,"annotations":null` {
				if conversionErr != nil {
					t.Fatalf("null metadata conversion changed: %v", conversionErr)
				}
			} else if !errors.Is(conversionErr, ErrUnsupported) {
				t.Fatalf("conversion boundary changed: %v", conversionErr)
			}
			u, err := ValidateProbeResponse(raw, "responses")
			if err != nil || !u.Known || u.Input != 3 || u.Output != 16 {
				t.Fatalf("valid metadata rejected: usage=%+v err=%v", u, err)
			}
		})
	}
}

func TestProbeValidationReasons(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, body, reason string
	}{
		{"json", "responses", `not json`, "invalid_json"},
		{"id", "responses", `{"output":[]}`, "missing_response_id"},
		{"error", "responses", `{"id":"r","error":{"message":"private upstream detail"}}`, "upstream_error"},
		{"status", "responses", `{"id":"r","status":"failed"}`, "upstream_error"},
		{"output", "responses", `{"id":"r","output":[{"type":"private unknown type"}]}`, "unsupported_output"},
		{"content", "responses", `{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"OK","private field name":"private value"}]}]}`, "unsupported_content"},
		{"logprobs", "responses", `{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"OK","logprobs":{}}]}]}`, "unsupported_content"},
		{"annotations", "responses", `{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"OK","annotations":"private detail"}]}]}`, "unsupported_content"},
		{"text", "responses", `{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":42}]}]}`, "unsupported_content"},
		{"tool", "responses", `{"id":"r","output":[{"type":"function_call","call_id":"c","name":"f","arguments":"invalid"}]}`, "invalid_tool_call"},
		{"choices", "chat-completions", `{"id":"r","choices":[]}`, "invalid_choices"},
		{"refusal", "chat-completions", `{"id":"r","choices":[{"message":{"refusal":"private refusal"}}]}`, "refusal"},
		{"anthropic-output", "anthropic", `{"id":"r","content":[{"type":"private unknown type"}]}`, "unsupported_output"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateProbeResponse([]byte(tc.body), tc.protocol)
			if err == nil || ResponseValidationReason(err) != tc.reason {
				t.Fatalf("reason=%q err=%v; want %q", ResponseValidationReason(err), err, tc.reason)
			}
		})
	}
	if ResponseValidationReason(nil) != "" || ResponseValidationReason(errors.New("private detail")) != "invalid_structure" {
		t.Fatal("fallback reason leaked arbitrary error")
	}
}
