package protocol

// Native Responses streams can carry generated content in snapshots and done
// events without sending the corresponding deltas. Recognize only known content
// fields; empty containers, IDs, statuses and unknown metadata are not output.
func responsesGeneratedContent(m Object) bool {
	switch m["type"] {
	case "response.reasoning_summary_text.done", "response.reasoning_text.done", "response.output_text.done":
		return str(m["text"]) != ""
	case "response.refusal.delta":
		return str(m["delta"]) != ""
	case "response.refusal.done":
		return str(m["refusal"]) != ""
	case "response.function_call_arguments.done":
		return str(m["arguments"]) != ""
	case "response.custom_tool_call_input.done":
		return str(m["input"]) != ""
	case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done", "response.content_part.added", "response.content_part.done":
		return responsesPartHasGeneratedContent(obj(m["part"]))
	case "response.output_item.added", "response.output_item.done":
		return responsesItemHasGeneratedContent(obj(m["item"]))
	case "response.completed", "response.incomplete":
		for _, v := range arr(obj(m["response"])["output"]) {
			if responsesItemHasGeneratedContent(obj(v)) {
				return true
			}
		}
	}
	return false
}

func responsesPartHasGeneratedContent(part Object) bool {
	switch part["type"] {
	case "summary_text", "reasoning_text", "output_text", "text":
		return str(part["text"]) != ""
	case "refusal":
		return str(part["refusal"]) != ""
	}
	return false
}

func responsesItemHasGeneratedContent(item Object) bool {
	switch item["type"] {
	case "reasoning":
		if str(item["encrypted_content"]) != "" {
			return true
		}
		for _, field := range []string{"summary", "content"} {
			for _, v := range arr(item[field]) {
				if responsesPartHasGeneratedContent(obj(v)) {
					return true
				}
			}
		}
	case "message":
		for _, v := range arr(item["content"]) {
			if responsesPartHasGeneratedContent(obj(v)) {
				return true
			}
		}
	case "function_call":
		return str(item["arguments"]) != ""
	case "custom_tool_call":
		return str(item["input"]) != ""
	}
	return false
}
