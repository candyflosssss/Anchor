package protocol

import "encoding/json"

const defaultAnthropicOutputTokens int64 = 4096

// OutputTokenPolicy is supplied from the selected mapping's saved declarations,
// never inferred from its name or fetched from a remote catalog during dispatch.
// Maximum is only a fallback for Messages' required max_tokens when the client
// supplied no budget and the mapping supplied no explicit default.
type OutputTokenPolicy struct {
	Default int64
	Maximum int64
}

func prepareAnthropicOutputBudget(fields map[string]json.RawMessage, native bool, policy OutputTokenPolicy) (int64, string) {
	field := "max_output_tokens"
	if native {
		field = "max_tokens"
	}
	if raw, exists := fields[field]; exists && string(raw) != "null" {
		var limit int64
		if json.Unmarshal(raw, &limit) != nil || limit < 1 {
			return 0, "client"
		}
		return limit, "client"
	}
	limit, source := defaultAnthropicOutputTokens, "legacy_fallback"
	if policy.Default > 0 {
		limit, source = policy.Default, "model_default"
	} else if policy.Maximum > 0 {
		limit, source = policy.Maximum, "model_limit"
	}
	fields[field], _ = json.Marshal(limit)
	return limit, source
}
