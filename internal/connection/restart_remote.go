package connection

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
)

// RestartCore invokes the enrolled account's identity-pinned restart operation.
// The SSH channel survives core handoff. No bootstrap, enrollment, token renewal,
// fallback target, ordinary stop, process kill or mutation retry is performed.
func (l *Live) RestartCore(ctx context.Context) (client.Instance, error) {
	var empty client.Instance
	if l.Profile.Transport != "ssh" || l.Profile.Validate() != nil {
		return empty, failure("invalid")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cl, info, err := l.Connect(ctx)
	if err != nil {
		return empty, err
	}
	cl.Close()
	if !slices.Contains(info.Capabilities, "restart_handoff") {
		return empty, failure("unsupported")
	}
	// The remote executable checks both pins again against its state before stop.
	seconds := 30
	if deadline, ok := ctx.Deadline(); ok {
		seconds = min(seconds, int(time.Until(deadline)/time.Second))
	}
	if seconds < 1 {
		return empty, failure("startup_failed")
	}
	stdout, stderr, err := l.restartCommand(ctx, remoteRestartCommand(l.Profile, seconds))
	if err != nil {
		for _, code := range []string{"remote_anchor_missing", "remote_anchor_unsupported"} {
			for _, line := range strings.Split(stderr, "\n") {
				if strings.TrimSpace(line) == "anchor-bootstrap: "+code || strings.TrimSpace(line) == "anchor-restart: "+code {
					return empty, failure(code)
				}
			}
		}
		return empty, unknownRestart()
	}
	var result struct {
		Ready      bool   `json:"ready"`
		InstanceID string `json:"instance_id"`
		ProcessID  int    `json:"process_id"`
		Recovered  bool   `json:"recovered"`
	}
	decoder := json.NewDecoder(bytes.NewBufferString(stdout))
	decoder.DisallowUnknownFields()
	if len(stdout) > 65536 || decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || !result.Ready || result.InstanceID != l.Profile.InstanceID || result.ProcessID <= 0 {
		return empty, unknownRestart()
	}
	cl, info, err = l.Connect(ctx)
	if err != nil {
		return empty, unknownRestart()
	}
	defer cl.Close()
	status, err := cl.Status(ctx)
	if err != nil || status["core"] != "ready" || info.ProcessID != result.ProcessID {
		return empty, unknownRestart()
	}
	return info, nil
}
func unknownRestart() error { return &client.RequestError{Code: "unknown", Unknown: true} }

func remoteRestartCommand(c state.Context, seconds int) string {
	const script = remoteExecutableDiscovery + `help=$("$executable" restart --help 2>&1 || :)
case "$help" in
  *-expected-instance*) ;;
  *) printf '%s\n' 'anchor-restart: remote_anchor_unsupported' >&2; exit 78 ;;
esac
exec "$executable" restart --approve --expected-instance "$3" --control-port "$2" --timeout "$4"`
	return "sh -c " + shellQuote(script) + " anchor-restart " + shellQuote(c.RemoteExecutable) + " " + strconv.Itoa(c.RemoteControlPort) + " " + shellQuote(c.InstanceID) + " " + strconv.Itoa(seconds) + "s"
}
func (l *Live) restartCommand(ctx context.Context, command string) (string, string, error) {
	stdout, stderr := &boundedBuffer{limit: 65537}, &boundedBuffer{limit: 16384}
	if l.native != nil {
		// A stalled SSH channel must not outlive the operation deadline. This only
		// closes our management tunnel, never a remote process or service.
		stop := context.AfterFunc(ctx, func() { l.native.Close() })
		defer stop()
		session, err := l.native.NewSession()
		if err != nil {
			return "", "", err
		}
		defer session.Close()
		session.Stdout, session.Stderr = stdout, stderr
		err = session.Run(command)
		return stdout.String(), stderr.String(), err
	}
	path, err := sshBinary()
	if err != nil {
		return "", "", err
	}
	args := append(sshArgs(l.Profile), "--", l.Profile.SSHTarget, command)
	cmd := exec.CommandContext(ctx, path, args...)
	hideWindow(cmd)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}
