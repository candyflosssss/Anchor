package protocol

import (
	"encoding/json"
	"strings"
	"unicode"
)

// RawServiceTier keeps the scalar JSON value independently of interpretation.
// Do not log objects, unbounded values, control characters or recognizable credentials.
// Unknown short values are retained (e.g. a provider's future tier name).
func RawServiceTier(value any, secrets ...string) string {
	switch v := value.(type) {
	case string:
		if len(v) > 128 || strings.ContainsFunc(v, unicode.IsControl) {
			return ""
		}
		lower := strings.ToLower(v)
		for _, marker := range []string{"sk-", "sk_", "bearer ", "ghp_", "github_pat_", "akia", "api_key", "api-key", "token=", "secret="} {
			if strings.Contains(lower, marker) {
				return ""
			}
		}
		for _, secret := range secrets {
			if secret != "" && strings.Contains(v, secret) {
				return ""
			}
		}
	case nil, bool, float64, json.Number:
	default:
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 256 {
		return ""
	}
	return string(raw)
}

// SanitizeRawServiceTier preserves scalar JSON values without rounding numbers.
func SanitizeRawServiceTier(raw []byte, secrets ...string) string {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if !json.Valid(raw) || decoder.Decode(&value) != nil {
		return ""
	}
	return RawServiceTier(value, secrets...)
}

// ReadRawServiceTier reads only service_tier, independently of protocol decoding.
func ReadRawServiceTier(raw []byte, p string, secrets ...string) string {
	if p != "responses" && p != "chat-completions" {
		return ""
	}
	var fields struct {
		Tier json.RawMessage `json:"service_tier"`
	}
	if json.Unmarshal(raw, &fields) != nil {
		return ""
	}
	return SanitizeRawServiceTier(fields.Tier, secrets...)
}

// SafeServiceTier returns only fixed diagnostic values, never arbitrary input.
// A requested tier is not proof of the tier actually selected by the upstream.
func SafeServiceTier(value any) string {
	switch value {
	case "auto", "default", "priority", "fast", "ultrafast", "flex", "scale":
		return value.(string)
	default:
		return "unknown"
	}
}

// ReadServiceTier reads OpenAI-compatible response metadata before conversion.
// Missing metadata is unknown, not evidence of Standard processing.
func ReadServiceTier(m Object, p string) string {
	if p != "responses" && p != "chat-completions" {
		return ""
	}
	if value, ok := m["service_tier"]; ok {
		return SafeServiceTier(value)
	}
	return ""
}

func (o *Observation) observeServiceTier(m Object, p string, raw []byte) {
	if o == nil {
		return
	}
	if p == "responses" {
		m = obj(m["response"])
	}
	if tier := ReadServiceTier(m, p); tier != "" {
		o.ServiceTier = tier
		// Avoid decoding every text/tool delta again when it has no tier metadata.
		if p == "responses" {
			var event struct {
				Response json.RawMessage `json:"response"`
			}
			_ = json.Unmarshal(raw, &event)
			raw = event.Response
		}
		o.ServiceTierRaw = ReadRawServiceTier(raw, p)
	}
}
