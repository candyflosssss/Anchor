// Package naming defines the shared contract for user-supplied identifiers.
package naming

import (
	"regexp"
	"strings"
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

// ValidModelID additionally permits the reserved speed-variant suffixes. Other
// user identifiers retain their original contract; nested/arbitrary parentheses
// are not accepted and the full public model ID stays within 128 code points.
func ValidModelID(s string) bool {
	if ValidID(s) {
		return true
	}
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > 128 {
		return false
	}
	for _, suffix := range []string{"(fast)", "(ultrafast)"} {
		if strings.HasSuffix(s, suffix) {
			return ValidID(strings.TrimSuffix(s, suffix))
		}
	}
	return false
}
