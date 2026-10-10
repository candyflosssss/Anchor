// Package branding contains the desktop icon; no runtime file dependency.
package branding

import _ "embed"

//go:embed anchor.png
var Icon []byte
