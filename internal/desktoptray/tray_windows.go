//go:build desktop && windows

package desktoptray

import "github.com/candyflosssss/Anchor/desktop/branding"

func icon() []byte            { return branding.WindowsIcon }
func platformSupported() bool { return true }
func platformAvailable() bool { return true }
