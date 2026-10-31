package protocol

import "maps"

// Messages cannot verify another protocol's reasoning ciphertext or accept its
// unsigned thought history. Project only this attempt's history, never the
// client's stored conversation or another candidate's baseline. Visible messages
// and tool calls/results remain intact; any lost reasoning is diagnosed below.
func prepareAnthropicHistory(m Object) error {
	input, ok := m["input"].([]any)
	if !ok {
		return nil
	} // The shared encoder validates other input shapes.
	kept := make([]any, 0, len(input))
	for _, value := range input {
		item := obj(value)
		switch item["type"] {
		case "reasoning":
			if _, err := anthropicReasoningHistoryLoss(item); err != nil {
				return err
			}
			continue
		case nil, "", "message":
			if phase := item["phase"]; phase != nil {
				if item["role"] != "assistant" || (phase != "commentary" && phase != "final_answer") {
					return conversionFailure("request_content", ErrUnsupported)
				}
			}
			delete(item, "phase")
		}
		kept = append(kept, value)
	}
	m["input"] = kept
	return nil
}

func anthropicReasoningHistoryLoss(item Object) (bool, error) {
	for _, key := range []string{"id", "status"} {
		if value := item[key]; value != nil {
			if _, ok := value.(string); !ok {
				return false, conversionFailure("reasoning_history", ErrUnsupported)
			}
		}
	}
	ciphertext := ""
	if value := item["encrypted_content"]; value != nil {
		var ok bool
		ciphertext, ok = value.(string)
		if !ok {
			return false, conversionFailure("encrypted_reasoning_history", ErrUnsupported)
		}
	}
	// Reuse the existing strict summary/content validator. Only the well-typed
	// opaque ciphertext is excluded from that validator's cross-protocol check.
	checked := maps.Clone(item)
	delete(checked, "encrypted_content")
	thought, err := chatHistoryReasoning(checked)
	if err != nil {
		return false, err
	}
	return thought != "" || ciphertext != "", nil
}

func anthropicHistoryDegradations(m Object) []Degradation {
	reasoning, phase, imageDetail := false, false, false
	for _, value := range arr(m["input"]) {
		item := obj(value)
		switch item["type"] {
		case "reasoning":
			lost, err := anthropicReasoningHistoryLoss(item)
			reasoning = reasoning || (err == nil && lost)
		case nil, "", "message":
			phase = phase || item["phase"] != nil
			imageDetail = imageDetail || anthropicImageDetailLost(item["content"])
		case "function_call_output", "custom_tool_call_output":
			imageDetail = imageDetail || anthropicImageDetailLost(item["output"])
		}
	}
	var losses []Degradation
	if reasoning {
		losses = append(losses, ReasoningHistoryOmitted)
	}
	if phase {
		losses = append(losses, MessagePhaseOmitted)
	}
	if imageDetail {
		losses = append(losses, ImageDetailOmitted)
	}
	return losses
}
