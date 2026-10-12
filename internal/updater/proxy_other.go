//go:build !windows

package updater

import "os"

var lookupEnv = os.Getenv

// Non-Windows hosts use the standard proxy environment variables only.
func readSystemProxy() (string, string, bool) { return "", "", false }
