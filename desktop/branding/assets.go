// Package branding contains the desktop icon; no runtime file dependency.
package branding

import _ "embed"

// Regenerate the checked-in Windows COFF resource after changing anchor.ico.
// Wails loads icon group ID 3 from the executable, independently of the tray.
//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --in windows-resource.json --out ../../cmd/anchor-desktop/rsrc --arch amd64

//go:embed anchor.png
var Icon []byte

//go:embed anchor.ico
var WindowsIcon []byte
