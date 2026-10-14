package connection

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestSafeSSHArgsAndErrors(t *testing.T) {
	c := state.Context{SSHTarget: "user@host", SSHPort: 2222, SSHIdentityFile: "/path with spaces/key", TrustNewHost: true}
	args := sshArgs(c)
	joined := strings.Join(args, "|")
	for _, want := range []string{"BatchMode=yes", "StrictHostKeyChecking=accept-new", "ExitOnForwardFailure=yes", "PermitLocalCommand=no", "-i|/path with spaces/key", "-p|2222"} {
		if !strings.Contains(joined, want) {
			t.Fatal(want, joined)
		}
	}
	for _, tc := range []struct{ text, code string }{{"WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED! synthetic-secret", "ssh_host_key"}, {"Permission denied (publickey)", "ssh_auth"}, {"connect to host example port 22: Connection refused", "ssh_unreachable"}, {"anchor: command not found synthetic-secret", "remote_anchor"}} {
		var err *Error
		if !errors.As(sshError(tc.text, "remote_anchor"), &err) || err.Code != tc.code || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal(tc, err)
		}
	}
}
func TestLocalConnectorAndBootstrapPreserveIdentity(t *testing.T) {
	s, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	srv := core.New(s, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	cfg, err := s.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860")
	if err != nil {
		t.Fatal(err)
	}
	live, err := Open(t.Context(), s, state.Context{Name: "local", Endpoint: web.URL, InstanceID: cfg.InstanceID, CredentialRef: "local-owner"})
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	cl, info, err := live.Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if info.InstanceID != cfg.InstanceID {
		t.Fatal("identity changed")
	}
	var port int
	_, err = fmt.Sscanf(cfg.ControlAddress, "127.0.0.1:%d", &port)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := EnsureLocal(context.Background(), s.Dir, port)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	next, _ := reopened.Settings()
	if next.InstanceID != cfg.InstanceID {
		t.Fatal("bootstrap replaced existing identity")
	}
	if _, err := EnsureLocal(t.Context(), s.Dir, port+1); err == nil {
		t.Fatal("mismatched port accepted")
	}
}
