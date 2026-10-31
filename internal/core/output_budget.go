package core

import (
	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
)

// Project saved mapping declarations only. Protocol-specific required fields and
// precedence stay in the protocol adapter; the gateway does not guess model caps.
func modelOutputTokenPolicy(model state.Model) protocol.OutputTokenPolicy {
	var policy protocol.OutputTokenPolicy
	if model.Settings == nil {
		return policy
	}
	configuration := model.Settings.Configuration
	if configuration.DefaultOutputTokens != nil {
		policy.Default = *configuration.DefaultOutputTokens
	}
	if configuration.Limits.Output != nil {
		policy.Maximum = *configuration.Limits.Output
	}
	return policy
}
