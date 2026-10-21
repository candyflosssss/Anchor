package protocol

import "encoding/json"

// AgentExecutedTool permits only client-executed Responses tools. Namespaces
// group function/custom tools; they do not grant hosted-tool capabilities.
func AgentExecutedTool(raw json.RawMessage, namespaced bool) bool {
	var tool struct {
		Type      string            `json:"type"`
		Name      string            `json:"name"`
		Execution string            `json:"execution"`
		Tools     []json.RawMessage `json:"tools"`
	}
	if json.Unmarshal(raw, &tool) != nil {
		return false
	}
	switch tool.Type {
	case "function", "custom":
		return true
	case "tool_search":
		return !namespaced && tool.Execution == "client"
	case "namespace":
		if namespaced || tool.Name == "" || len(tool.Tools) == 0 {
			return false
		}
		for _, child := range tool.Tools {
			if !AgentExecutedTool(child, true) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
