package protocol

import (
	"encoding/json"
	"strings"
)

// ClassifyUpstreamError returns only code-owned labels. The caller bounds the
// input and owns the response reader; remote messages and parameter names never
// escape into diagnostics. Classification does not authorize changing or replaying
// the request.
func ClassifyUpstreamError(data []byte) string {
	if len(data) > 16<<10 {
		return ""
	}
	var body struct {
		Error struct {
			Code    string          `json:"code"`
			Type    string          `json:"type"`
			Param   json.RawMessage `json:"param"`
			Message json.RawMessage `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &body) != nil {
		return ""
	}
	e := body.Error
	if e.Code == "unsupported_value" || e.Code == "invalid_value" || e.Code == "invalid_request_error" || (e.Code == "" && e.Type == "invalid_request_error") {
		var param, message string
		_ = json.Unmarshal(e.Param, &param)
		_ = json.Unmarshal(e.Message, &message)
		if ((e.Code == "unsupported_value" || e.Code == "invalid_value") && (param == "reasoning.effort" || param == "reasoning_effort" || param == "output_config.effort")) || unsupportedEffortMessage(message) {
			return "unsupported_reasoning_effort"
		}
	}
	known := map[string]string{
		"invalid_api_key": "invalid_api_key", "authentication_error": "invalid_api_key",
		"insufficient_quota": "insufficient_quota", "billing_hard_limit_reached": "insufficient_quota",
		"rate_limit_exceeded": "rate_limit", "rate_limit_error": "rate_limit",
		"model_not_found": "model_not_found", "permission_denied": "permission_denied", "permission_error": "permission_denied",
		"context_length_exceeded": "context_length", "invalid_request_error": "invalid_request",
		"overloaded_error": "overloaded", "server_error": "server_error", "api_error": "server_error",
	}
	for _, value := range []string{e.Code, e.Type} {
		if category, ok := known[value]; ok {
			return category
		}
	}
	return ""
}

// Some relays omit error.param/code. Recognize only this complete error
// template with known effort values, never arbitrary mentions of reasoning.
func unsupportedEffortMessage(message string) bool {
	rest, ok := strings.CutPrefix(message, "Unsupported value: '")
	if !ok {
		return false
	}
	value, rest, ok := strings.Cut(rest, "' is not supported with the '")
	if !ok || !chatReasoningEffort(value) {
		return false
	}
	model, rest, ok := strings.Cut(rest, "' model. Supported values are: ")
	if !ok || model == "" || len(model) > 256 || strings.ContainsAny(model, "\r\n'") {
		return false
	}
	rest = strings.TrimSuffix(rest, ".")
	rest = strings.NewReplacer(", and ", ", ", " and ", ", ").Replace(rest)
	values := strings.Split(rest, ", ")
	if len(values) == 0 || len(values) > 8 {
		return false
	}
	for _, v := range values {
		v, ok = strings.CutPrefix(v, "'")
		if !ok {
			return false
		}
		v, ok = strings.CutSuffix(v, "'")
		if !ok || !chatReasoningEffort(v) || v == value {
			return false
		}
	}
	return true
}
