//go:build !linux && !windows

package egress

import "embed"

var bundle embed.FS

const bundlePath = ""
const bundleHash = ""
