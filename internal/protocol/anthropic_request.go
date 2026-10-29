package protocol

// prepareAnthropicIngress removes only understood cache hints from the private
// conversion copy. Native dispatch always uses Session.native. The corresponding
// degradation is derived from that original payload, never from this projection.
func prepareAnthropicIngress(m, out Object) error {
	if metadata := m["metadata"]; metadata != nil {
		v := obj(metadata)
		if allowed(v, "user_id") != nil {
			return conversionFailure("request_fields", ErrUnsupported)
		}
		if user := v["user_id"]; user != nil {
			if _, ok := user.(string); !ok {
				return conversionFailure("request_fields", ErrUnsupported)
			}
			out["user"] = user
		}
	}
	var strip func(any) error
	strip = func(value any) error {
		for _, v := range arr(value) {
			b := obj(v)
			if hint := b["cache_control"]; hint != nil {
				cache := obj(hint)
				if allowed(cache, "type ttl") != nil || cache["type"] != "ephemeral" ||
					(cache["ttl"] != nil && cache["ttl"] != "5m" && cache["ttl"] != "1h") {
					return conversionFailure("request_content", ErrUnsupported)
				}
			}
			delete(b, "cache_control")
			if b["type"] == "tool_result" {
				if err := strip(b["content"]); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := strip(m["system"]); err != nil {
		return err
	}
	if err := strip(m["tools"]); err != nil {
		return err
	}
	for _, v := range arr(m["messages"]) {
		if err := strip(obj(v)["content"]); err != nil {
			return err
		}
	}
	return nil
}

func anthropicIngressDegradations(raw []byte) []Degradation {
	m, _ := Decode(raw)
	cache, toolError := false, false
	var visit func(any)
	visit = func(value any) {
		for _, v := range arr(value) {
			b := obj(v)
			cache = cache || b["cache_control"] != nil
			if b["type"] == "tool_result" {
				toolError = toolError || b["is_error"] == true
				visit(b["content"])
			}
		}
	}
	visit(m["system"])
	visit(m["tools"])
	for _, v := range arr(m["messages"]) {
		visit(obj(v)["content"])
	}
	var d []Degradation
	if cache {
		d = append(d, PromptCacheControl)
	}
	if toolError {
		d = append(d, ToolResultErrorText)
	}
	return d
}
