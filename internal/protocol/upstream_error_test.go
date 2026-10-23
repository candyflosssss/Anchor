package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClassifyUnsupportedReasoningEffort(t *testing.T) {
	const message = "Unsupported value: 'none' is not supported with the 'private-model' model. Supported values are: 'low', 'medium', 'high', 'xhigh', and 'max'."
	for _, tc := range []struct {
		name, code, typ, param, message, want string
	}{
		{"relay-message", "", "invalid_request_error", "", message, "unsupported_reasoning_effort"},
		{"structured-responses", "unsupported_value", "invalid_request_error", "reasoning.effort", "private-input", "unsupported_reasoning_effort"},
		{"structured-chat", "invalid_value", "", "reasoning_effort", "private-input", "unsupported_reasoning_effort"},
		{"structured-anthropic", "unsupported_value", "", "output_config.effort", "private-input", "unsupported_reasoning_effort"},
		{"wrong-param", "unsupported_value", "invalid_request_error", "private-param", "private-input", "invalid_request"},
		{"generic-mention", "", "invalid_request_error", "", "private-input reasoning.effort none", "invalid_request"},
		{"unknown-value", "", "invalid_request_error", "", strings.Replace(message, "'none'", "'private-input'", 1), "invalid_request"},
		{"unknown-supported-value", "", "invalid_request_error", "", strings.Replace(message, "'max'", "'private-input'", 1), "invalid_request"},
		{"not-effort-values", "", "invalid_request_error", "", "Unsupported value: 'none' is not supported with the 'private-model' model. Supported values are: 'auto' and 'required'.", "invalid_request"},
		{"prefixed-prompt", "", "invalid_request_error", "", "private-input " + message, "invalid_request"},
		{"suffixed-prompt", "", "invalid_request_error", "", message + " private-input", "invalid_request"},
		{"not-error-envelope", "", "", "", message, ""},
		{"specific-code-wins", "context_length_exceeded", "invalid_request_error", "", message, "context_length"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"error": map[string]any{"code": tc.code, "type": tc.typ, "param": tc.param, "message": tc.message}})
			if got := ClassifyUpstreamError(body); got != tc.want {
				t.Fatalf("category=%q want=%q", got, tc.want)
			}
		})
	}
	if got := ClassifyUpstreamError([]byte(`{"error":{"type":"invalid_request_error","message":"` + message + strings.Repeat("x", 16<<10) + `"}}`)); got != "" {
		t.Fatal("oversized error classified", got)
	}
	if got := ClassifyUpstreamError([]byte(`{"message":"` + message + `"}`)); got != "" {
		t.Fatal("non-error text classified", got)
	}
}
