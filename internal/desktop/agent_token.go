package desktop

import "github.com/candyflosssss/Anchor/internal/state"

// IsAgentTokenInvocation keeps the desktop executable usable as the shared
// compatibility layer's credential helper without exposing general GUI shell use.
func IsAgentTokenInvocation(args []string) bool {
	return len(args) == 5 && args[0] == "--data-dir" && args[1] != "" && args[2] == "agent" && args[3] == "token" && state.ValidID(args[4])
}
