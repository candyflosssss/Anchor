//go:build linux

package egress

import "embed"

//go:embed payload/linux
var bundle embed.FS

const bundlePath = "payload/linux/mihomo.gz"
const bundleHash = "ba3ce607747a07f948fc35780e108a4a7c7f552a38b9bd4d115f313ebcb89c20"
