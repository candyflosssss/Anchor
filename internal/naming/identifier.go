// Package naming defines the shared contract for user-supplied identifiers.
package naming

import (
	"regexp"
	"unicode/utf8"
)

var identifier = regexp.MustCompile(`^[\p{L}\p{Nd}][\p{L}\p{Nd}._-]{0,127}$`)
var asciiIdentifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

// ValidID accepts 1..128 Unicode code points, without normalization or trimming.
// Whitespace, path separators, control/format characters and URL delimiters
// are excluded. The first character must be a letter or decimal digit.
func ValidID(s string) bool { return utf8.ValidString(s) && identifier.MatchString(s) }

// ValidCode preserves the ASCII contract for machine-readable error codes.
func ValidCode(s string) bool { return asciiIdentifier.MatchString(s) }
