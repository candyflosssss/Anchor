//go:build desktop

package frontend

import "embed"

// Build the production frontend before compiling the desktop host.
//
//go:embed all:dist
var Assets embed.FS
