package protocol

// OrphanToolOutputText marks a Responses tool output without an earlier call in
// the same input that was sent as user content: its tool identity is lost.
const OrphanToolOutputText Degradation = "orphan_tool_output_text"

// orphanToolOutput converts an unpaired function/custom tool output for Chat or
// Messages, which both reject tool results without a matching call. The output
// text (and, for Messages, validated images) is kept unchanged; name/namespace
// are only accepted here as client identity strings and are never forwarded.
// A nil result means the output was empty and carries no content.
func orphanToolOutput(item Object, to string) (any, error) {
	if err := allowed(item, "type id call_id name namespace output status"); err != nil {
		return nil, err
	}
	for _, key := range []string{"id", "call_id", "name", "namespace", "status"} {
		if v := item[key]; v != nil {
			if _, ok := v.(string); !ok {
				return nil, ErrUnsupported
			}
		}
	}
	if to == "anthropic" {
		content, err := anthropicContent(item["output"])
		if err != nil {
			return nil, err
		}
		if t, ok := content.(string); ok && t == "" {
			return nil, nil
		}
		return content, nil
	}
	t, err := text(item["output"])
	if err != nil || t == "" {
		return nil, err
	}
	return t, nil
}

// orphanToolOutputDegradations uses the same pairing rule as fromResponses:
// a result pairs only with a function/custom call earlier in this input.
func orphanToolOutputDegradations(raw []byte, to string) []Degradation {
	if to != "chat-completions" && to != "anthropic" {
		return nil
	}
	m, _ := Decode(raw)
	called := map[string]bool{}
	for _, v := range arr(m["input"]) {
		item := obj(v)
		switch item["type"] {
		case "function_call", "custom_tool_call":
			if id := str(item["call_id"]); id != "" {
				called[id] = true
			}
		case "function_call_output", "custom_tool_call_output":
			if id := str(item["call_id"]); id == "" || !called[id] {
				return []Degradation{OrphanToolOutputText}
			}
		}
	}
	return nil
}
