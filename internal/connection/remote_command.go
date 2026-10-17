package connection

import (
	"strconv"
	"strings"

	"github.com/candyflosssss/Anchor/internal/state"
)

// remoteExecutableDiscovery runs in a fresh POSIX shell without sourcing login files.
// Both authentication modes share discovery and diagnostics. A custom executable
// is authoritative; it never silently falls back to another installation.
const remoteExecutableDiscovery = `requested=$1
case "$requested" in
  anchor)
    executable=$(command -v anchor 2>/dev/null) || executable=
    if [ -z "$executable" ]; then
      for candidate in "$HOME/.local/bin/anchor" "$HOME/bin/anchor" /usr/local/bin/anchor /usr/bin/anchor; do
        if [ -x "$candidate" ] && [ ! -d "$candidate" ]; then executable=$candidate; break; fi
      done
    fi
    ;;
  "~/"*) executable="$HOME/${requested#\~/}" ;;
  *) executable=$(command -v "$requested" 2>/dev/null) || executable= ;;
esac
if [ -z "$executable" ] || [ ! -x "$executable" ] || [ -d "$executable" ]; then
  printf '%s\n' 'anchor-bootstrap: remote_anchor_missing' >&2
  exit 127
fi
`

func bootstrapCommand(c state.Context) string {
	const script = remoteExecutableDiscovery + `if ! "$executable" context bootstrap --help >/dev/null 2>&1; then
  printf '%s\n' 'anchor-bootstrap: remote_anchor_unsupported' >&2
  exit 78
fi
exec "$executable" context bootstrap --control-port "$2" --show-once`
	return "sh -c " + shellQuote(script) + " anchor-bootstrap " + shellQuote(c.RemoteExecutable) + " " + strconv.Itoa(c.RemoteControlPort)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

// Never return raw remote stderr, which can contain secrets or terminal control
// characters. Recognize only our fixed diagnostics, retaining a generic fallback
// for older remote binaries and unexpected failures.
func remoteBootstrapError(stderr, fallback string) error {
	for _, code := range []string{"remote_anchor_missing", "remote_anchor_unsupported"} {
		for _, line := range strings.Split(stderr, "\n") {
			if strings.TrimSpace(line) == "anchor-bootstrap: "+code {
				return failure(code)
			}
		}
	}
	for _, code := range []string{"remote_control_port", "remote_core_not_ready"} {
		if strings.Contains(stderr, "anchor: connection failed ("+code+")") {
			return failure(code)
		}
	}
	return failure(fallback)
}
