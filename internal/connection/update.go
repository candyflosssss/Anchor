package connection

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/candyflosssss/Anchor/internal/updater"
)

func remoteUpdateCommand(c state.Context, operation string, checked updater.Check) string {
	const script = remoteExecutableDiscovery + `help=$("$executable" update upgrade --help 2>&1 || :)
case "$help" in
 *-expected-sha256*) ;;
 *) printf '%s\n' 'anchor-bootstrap: remote_anchor_unsupported' >&2; exit 78 ;;
esac
operation=$4
if [ "$operation" = check ]; then
 exec "$executable" update check --expected-instance "$3" --control-port "$2"
fi
exec "$executable" update upgrade --approve --expected-instance "$3" --control-port "$2" --expected-version "$5" --expected-sha256 "$6"`
	hash := ""
	if checked.Artifact != nil {
		hash = checked.Artifact.SHA256
	}
	return "sh -c " + shellQuote(script) + " anchor-update " + shellQuote(c.RemoteExecutable) + " " + strconv.Itoa(c.RemoteControlPort) + " " + shellQuote(c.InstanceID) + " " + shellQuote(operation) + " " + shellQuote(checked.Version) + " " + shellQuote(hash)
}

func decodeUpdate(raw string, out any) error {
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if len(raw) > 65536 || d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return unknownRestart()
	}
	return nil
}

// CheckCoreUpdate uses the remote account's source/configuration, never the
// controlling computer's artifact or platform. No mutation is sent to the core.
func (l *Live) CheckCoreUpdate(ctx context.Context) (updater.Check, error) {
	var out updater.Check
	if l.Profile.Transport != "ssh" || l.Profile.Validate() != nil {
		return out, failure("invalid")
	}
	cl, info, err := l.Connect(ctx)
	if err != nil {
		return out, err
	}
	cl.Close()
	raw, stderr, err := l.restartCommand(ctx, remoteUpdateCommand(l.Profile, "check", out))
	if err != nil {
		return out, sshError(stderr, "remote_anchor")
	}
	if err := decodeUpdate(raw, &out); err != nil {
		return out, err
	}
	if out.Product != "cli" || out.CurrentVersion != info.Version || !updater.ValidChannel(out.Channel) || (out.Available && (out.Artifact == nil || out.Artifact.Kind != "binary")) {
		return updater.Check{}, failure("remote_anchor_unsupported")
	}
	return out, nil
}

func (l *Live) UpgradeCore(ctx context.Context, checked updater.Check) (updater.RuntimeResult, error) {
	var out updater.RuntimeResult
	if l.Profile.Transport != "ssh" || l.Profile.Validate() != nil || !checked.Available || checked.Product != "cli" || checked.Artifact == nil {
		return out, failure("invalid")
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	// Confirm the live authenticated instance immediately before the SSH mutation.
	cl, _, err := l.Connect(ctx)
	if err != nil {
		return out, err
	}
	cl.Close()
	raw, stderr, err := l.restartCommand(ctx, remoteUpdateCommand(l.Profile, "upgrade", checked))
	if err != nil {
		if strings.Contains(stderr, "anchor-bootstrap: remote_anchor_unsupported") || strings.Contains(stderr, "anchor-bootstrap: remote_anchor_missing") {
			return out, remoteBootstrapError(stderr, "remote_anchor")
		}
		return out, unknownRestart() // SSH loss does not prove that a mutation failed.
	}
	if err := decodeUpdate(raw, &out); err != nil {
		return out, err
	}
	if !out.Ready || out.InstanceID != l.Profile.InstanceID || out.Version != checked.Version || out.ProcessID < 1 {
		return out, unknownRestart()
	}
	verify, cancelVerify := context.WithTimeout(ctx, 10*time.Second)
	defer cancelVerify()
	cl, info, err := l.Connect(verify)
	if err != nil {
		return out, unknownRestart()
	}
	defer cl.Close()
	status, err := cl.Status(verify)
	if err != nil || status["core"] != "ready" || info.Version != out.Version || info.ProcessID != out.ProcessID {
		return out, &client.RequestError{Code: "unknown", Unknown: true}
	}
	return out, nil
}
