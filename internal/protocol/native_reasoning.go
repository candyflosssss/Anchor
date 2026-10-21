package protocol

import "encoding/json"

// Sanitize only the dispatch copy. Ciphertext is opaque: nonempty strings,
// summaries, unknown fields and all non-reasoning items retain their values.
func sanitizeNativeReasoningInput(input json.RawMessage) json.RawMessage {
	var items []json.RawMessage
	if json.Unmarshal(input, &items) != nil {
		return input
	}
	changed := false
	for i, raw := range items {
		var item map[string]json.RawMessage
		if json.Unmarshal(raw, &item) != nil {
			continue
		}
		var kind string
		if json.Unmarshal(item["type"], &kind) != nil || kind != "reasoning" {
			continue
		}
		value, exists := item["encrypted_content"]
		if !exists {
			continue
		}
		var ciphertext string
		if json.Unmarshal(value, &ciphertext) == nil && ciphertext != "" {
			continue
		}
		delete(item, "encrypted_content")
		// Prepare forces store=false. Without ciphertext, an id can become an
		// unavailable upstream store lookup instead of a self-contained item.
		delete(item, "id")
		items[i], _ = json.Marshal(item)
		changed = true
	}
	if !changed {
		return input
	}
	cleaned, _ := json.Marshal(items)
	return cleaned
}
