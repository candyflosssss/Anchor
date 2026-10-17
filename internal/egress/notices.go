package egress

import _ "embed"

// License and source information stay with standalone executables too.
//
//go:embed payload/LICENSE
var License string

//go:embed payload/NOTICE.md
var Notice string
