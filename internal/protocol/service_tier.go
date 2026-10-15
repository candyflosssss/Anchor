package protocol

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

func (o *Observation) observeServiceTier(m Object, p string) {
	if o == nil {
		return
	}
	if p == "responses" {
		m = obj(m["response"])
	}
	if tier := ReadServiceTier(m, p); tier != "" {
		o.ServiceTier = tier
	}
}
