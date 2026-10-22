package core

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
)

// Extract only recognized error identities from a bounded JSON prefix. Restore
// every byte for the existing recovery detector. Remote messages are not logs.
func captureUpstreamError(resp *http.Response, evidence map[string]string) {
	reader := bufio.NewReaderSize(resp.Body, 16<<10)
	prefix, readErr := reader.Peek(16 << 10)
	resp.Body = &inferenceBody{Reader: reader, Closer: resp.Body}
	if readErr != nil && readErr != io.EOF {
		return
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(prefix, &body) != nil {
		return
	}
	known := map[string]string{
		"invalid_api_key": "invalid_api_key", "authentication_error": "invalid_api_key",
		"insufficient_quota": "insufficient_quota", "billing_hard_limit_reached": "insufficient_quota",
		"rate_limit_exceeded": "rate_limit", "rate_limit_error": "rate_limit",
		"model_not_found": "model_not_found", "permission_denied": "permission_denied", "permission_error": "permission_denied",
		"context_length_exceeded": "context_length", "invalid_request_error": "invalid_request",
		"overloaded_error": "overloaded", "server_error": "server_error", "api_error": "server_error",
	}
	for _, value := range []string{body.Error.Code, body.Error.Type} {
		if category, ok := known[value]; ok {
			evidence["upstream_error_category"] = category
			return
		}
	}
}
