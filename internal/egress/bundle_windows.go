//go:build windows

package egress

import "embed"

//go:embed payload/windows
var bundle embed.FS

const bundlePath = "payload/windows/mihomo.zip"
const bundleHash = "974a4d7ad69aed27aa2e8f91d61113573c14dadb14562c63e58effabf59816f0"
