package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Repair item IDs only in the Responses dispatch copy. call_id belongs to the
// client tool loop and must never change with the upstream's item ID format.
func sanitizeResponsesToolIDs(input json.RawMessage) json.RawMessage {
	var items []json.RawMessage
	if json.Unmarshal(input, &items) != nil {
		return input
	}
	type toolItem struct {
		Type json.RawMessage `json:"type"`
		ID   json.RawMessage `json:"id"`
	}
	occupied := make(map[string]bool, len(items))
	parsed := make([]toolItem, len(items))
	for i, raw := range items {
		// Decode the ID separately so unknown/non-string IDs pass through intact.
		if json.Unmarshal(raw, &parsed[i]) == nil {
			var id string
			if json.Unmarshal(parsed[i].ID, &id) == nil && id != "" {
				occupied[id] = true
			}
		}
	}
	mapped := make(map[string]string)
	changed := false
	for i, item := range parsed {
		var kind string
		if json.Unmarshal(item.Type, &kind) != nil {
			continue
		}
		var prefix string
		switch kind {
		case "function_call":
			prefix = "fc"
		case "custom_tool_call":
			prefix = "ctc"
		case "custom_tool_call_output":
			prefix = "ctco"
		default:
			continue
		}
		var original string
		if json.Unmarshal(item.ID, &original) != nil || original == "" {
			continue
		}
		id := original
		if !strings.HasPrefix(id, prefix) {
			id = prefix + "_" + id
		}
		if id == original && utf8.RuneCountInString(id) <= 64 {
			continue
		}
		key := kind + "\x00" + original
		if prior, ok := mapped[key]; ok {
			id = prior
		} else {
			base := id
			for attempt := 0; utf8.RuneCountInString(id) > 64 || occupied[id]; attempt++ {
				seed := base + "\x00" + strconv.Itoa(attempt)
				sum := sha256.Sum256([]byte(seed))
				runes := []rune(base)
				if len(runes) > 47 {
					runes = runes[:47]
				}
				id = string(runes) + "_" + hex.EncodeToString(sum[:8])
			}
			occupied[id] = true
			mapped[key] = id
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(items[i], &fields) != nil {
			continue
		}
		fields["id"], _ = json.Marshal(id)
		items[i], _ = json.Marshal(fields)
		changed = true
	}
	if !changed {
		return input
	}
	result, _ := json.Marshal(items)
	return result
}
