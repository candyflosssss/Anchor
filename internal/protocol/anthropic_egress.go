package protocol

// prepareAnthropicRequest handles Responses options before the shared message
// encoder. Do not turn model controls into guessed Claude budgets or silently
// discard history just to make a request dispatchable.
func prepareAnthropicRequest(m Object) (*ChatMapping, Object, error) {
	keys := "model input instructions stream temperature top_p tools tool_choice max_output_tokens store background parallel_tool_calls prompt_cache_key client_metadata stream_options include truncation text reasoning"
	if err := allowed(m, keys); err != nil {
		return nil, nil, conversionFailure("request_fields", err)
	}
	// This OpenAI cache-affinity hint has no Messages equivalent. Validate it
	// before omission; never repurpose it as cache_control or user metadata.
	if v := m["prompt_cache_key"]; v != nil {
		if _, ok := v.(string); !ok {
			return nil, nil, conversionFailure("request_fields", ErrUnsupported)
		}
	}
	delete(m, "prompt_cache_key")
	if v := m["client_metadata"]; v != nil {
		metadata := obj(v)
		if metadata == nil {
			return nil, nil, conversionFailure("request_fields", ErrUnsupported)
		}
		for _, value := range metadata {
			if _, ok := value.(string); !ok {
				return nil, nil, conversionFailure("request_fields", ErrUnsupported)
			}
		}
	}
	delete(m, "client_metadata")
	if v := m["stream_options"]; v != nil {
		opts := obj(v)
		if allowed(opts, "reasoning_summary_delivery") != nil ||
			(opts["reasoning_summary_delivery"] != nil && opts["reasoning_summary_delivery"] != "sequential_cutoff") {
			return nil, nil, conversionFailure("request_fields", ErrUnsupported)
		}
	}
	delete(m, "stream_options")
	if v := m["include"]; v != nil {
		list, ok := v.([]any)
		if !ok {
			return nil, nil, conversionFailure("include_options", ErrUnsupported)
		}
		for _, value := range list {
			if value != "reasoning.encrypted_content" {
				return nil, nil, conversionFailure("include_options", ErrUnsupported)
			}
		}
	}
	delete(m, "include")
	if v := m["truncation"]; v != nil && v != "disabled" {
		return nil, nil, conversionFailure("truncation", ErrUnsupported)
	}
	delete(m, "truncation")
	if v := m["text"]; v != nil {
		t := obj(v)
		if allowed(t, "format") != nil {
			return nil, nil, conversionFailure("text_format", ErrUnsupported)
		}
		if v := t["format"]; v != nil {
			f := obj(v)
			if allowed(f, "type") != nil || f["type"] != "text" {
				return nil, nil, conversionFailure("text_format", ErrUnsupported)
			}
		}
	}
	delete(m, "text")
	options := Object{}
	if v := m["reasoning"]; v != nil {
		r := obj(v)
		if allowed(r, "effort summary") != nil {
			return nil, nil, conversionFailure("reasoning_options", ErrUnsupported)
		}
		if effort := r["effort"]; effort != nil {
			if effort != "none" {
				return nil, nil, conversionFailure("reasoning_options", ErrUnsupported)
			}
			options["thinking"] = Object{"type": "disabled"}
		}
		if summary := r["summary"]; summary != nil && summary != "auto" && summary != "concise" && summary != "detailed" {
			return nil, nil, conversionFailure("reasoning_options", ErrUnsupported)
		}
	}
	delete(m, "reasoning")
	mapping, err := prepareMappedTools(m)
	return mapping, options, err
}

func anthropicEgressDegradations(raw []byte) []Degradation {
	m, _ := Decode(raw)
	var d []Degradation
	if str(m["prompt_cache_key"]) != "" {
		d = append(d, PromptCacheKeyOmitted)
	}
	if len(obj(m["client_metadata"])) > 0 {
		d = append(d, Degradation("client_metadata_omitted"))
	}
	if obj(m["stream_options"])["reasoning_summary_delivery"] != nil {
		d = append(d, Degradation("reasoning_summary_delivery"))
	}
	if obj(m["reasoning"])["summary"] != nil {
		d = append(d, SummaryStyle)
	}
	return d
}
