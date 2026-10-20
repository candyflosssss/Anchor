package desktop

import (
	"strings"

	"github.com/candyflosssss/Anchor/internal/state"
)

// CLIDataDir resolves the same global option forms as the shared CLI parser.
func CLIDataDir(args []string) (string, error) {
	dir, err := state.DefaultDir()
	if err != nil {
		return "", err
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "--data-dir" && i+1 < len(args) {
			i++
			dir = args[i]
		} else if strings.HasPrefix(args[i], "--data-dir=") {
			dir = strings.TrimPrefix(args[i], "--data-dir=")
		}
	}
	return dir, nil
}

// IsCLIInvocation recognizes only the headless commands needed by launchers and
// SSH account control; arbitrary GUI flags are never passed to the CLI parser.
func IsCLIInvocation(args []string) bool {
	for i := 0; i < len(args); i++ {
		if args[i] == "--data-dir" || args[i] == "--context" {
			i++
			continue
		}
		if strings.HasPrefix(args[i], "--data-dir=") || strings.HasPrefix(args[i], "--context=") || args[i] == "--json" || args[i] == "--ssh-password-stdin" {
			continue
		}
		switch args[i] {
		case "update", "version", "serve", "restart", "context", "stop":
			return true
		}
		return false
	}
	return false
}
