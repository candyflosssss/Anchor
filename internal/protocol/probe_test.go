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
