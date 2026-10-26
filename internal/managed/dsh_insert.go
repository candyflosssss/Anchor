package managed

import "gopkg.in/yaml.v3"

// The editor addresses top-level ID overrides, not the composed DSH entry tree.
// Preserve independent insertions, but never mistake an insertion for an owned
// override or let an inserted entry shadow either model plugin during loading.
// StructuredEditor.parse also enforces this boundary during recovery.
func validateDSHInserts(root *yaml.Node) error {
	managed := func(id string) bool { return id == "llm-pi-ai" || id == "agent-default-model" }
	literalID := func(row *yaml.Node) bool {
		id := nodeKey(row, "id")
		return id == nil || (id.Kind == yaml.ScalarNode && id.Tag == "!!str" && id.Value != "" && !managed(id.Value))
	}
	insertedIDs := map[string]bool{}
	insertedGroups := map[string]bool{}
	var entries func(*yaml.Node) bool
	entries = func(list *yaml.Node) bool {
		if list.Kind != yaml.SequenceNode || list.Tag != "!!seq" {
			return false
		}
		for _, entry := range list.Content {
			if entry.Kind != yaml.MappingNode || entry.Tag != "!!map" || !literalID(entry) || nodeKey(entry, "insert") != nil {
				return false
			}
			if name := nodeKey(entry, "name"); name != nil && (name.Kind != yaml.ScalarNode || name.Tag != "!!str" || name.Value == "") {
				return false
			}
			isGroup := false
			if id := nodeKey(entry, "id"); id != nil {
				if id.Value == "" || insertedIDs[id.Value] {
					return false
				}
				insertedIDs[id.Value] = true
			}
			if group := nodeKey(entry, "group"); group != nil {
				// Dynamic group expressions cannot be inspected without running DSH.
				if group.Kind != yaml.ScalarNode || group.Tag != "!!bool" {
					return false
				}
				if err := group.Decode(&isGroup); err != nil {
					return false
				}
				if isGroup {
					if id := nodeKey(entry, "id"); id != nil {
						insertedGroups[id.Value] = true
					}
					config := nodeKey(entry, "config")
					if config == nil || !entries(config) {
						return false
					}
				}
			}
			if !isGroup && nodeKey(entry, "name") == nil {
				return false
			}
		}
		return true
	}
	for _, row := range root.Content {
		insert := nodeKey(row, "insert")
		if insert == nil {
			continue
		}
		if !literalID(row) || !entries(insert) {
			return ErrUnsafe
		}
		// DSH handles insert before overrides; config/disabled/name on this
		// row would be ignored, making narrow field verification misleading.
		for i := 0; i < len(row.Content); i += 2 {
			if key := row.Content[i].Value; key != "id" && key != "insert" {
				return ErrUnsafe
			}
		}
	}
	// Later ID patches can turn opaque plugin data into group children. Keep
	// inserted group topology fixed rather than pretending to compose DSH.
	for _, row := range root.Content {
		if nodeKey(row, "insert") != nil {
			continue
		}
		if id := nodeKey(row, "id"); id != nil && insertedIDs[id.Value] {
			if nodeKey(row, "group") != nil || (insertedGroups[id.Value] && nodeKey(row, "config") != nil) {
				return ErrUnsafe
			}
		}
	}
	return nil
}
