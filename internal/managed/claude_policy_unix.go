//go:build !windows

package managed

import (
	"path/filepath"
	"runtime"
)

func claudeDesktopPolicyPresent(root string) bool {
	if runtime.GOOS == "darwin" {
		// root = ~/Library/Application Support/Claude-3p/configLibrary
		home := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(root))))
		return filePresentOrUnreadable("/Library/Managed Preferences/com.anthropic.claudefordesktop.plist") ||
			filePresentOrUnreadable(filepath.Join("/Library/Managed Preferences", filepath.Base(home), "com.anthropic.claudefordesktop.plist"))
	}
	return filePresentOrUnreadable("/etc/claude-desktop/managed-settings.json")
}
