package core

import (
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/protocol"
)

// Retain the existing regression suite while the gateway uses Session policy.
func agentExecutedTool(raw json.RawMessage, namespaced bool) bool {
	return protocol.AgentExecutedTool(raw, namespaced)
}
