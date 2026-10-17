package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/state"
)

// Real isolated SSH transport and management authentication, with only the remote
// restart process handoff simulated. Never invokes the user's SSH configuration.
func TestRemoteRestartIdentityAndTarget(t *testing.T) {
	for _, password := range []bool{false, true} {
		for _, mode := range []string{"success", "malformed", "identity", "pid", "failed", "old-cli"} {
			t.Run(fmt.Sprintf("password=%t/%s", password, mode), func(t *testing.T) {
				var calls atomic.Int32
				var remote *state.Store
				handler := func(args []string) (string, uint32) {
					calls.Add(1)
					cfg, err := remote.Settings()
					if err != nil {
						t.Error(err)
						return "", 1
					}
					port := strings.TrimPrefix(cfg.ControlAddress, "127.0.0.1:")
					if len(args) != 8 || strings.Join(args[:6], " ") != "restart --approve --expected-instance "+cfg.InstanceID+" --control-port "+port || args[6] != "--timeout" {
						t.Errorf("unsafe command arguments: %q", args)
					}
					if mode == "malformed" {
						return "not-json", 0
					}
					if mode == "failed" {
						return "", 1
					}
					id, pid := cfg.InstanceID, os.Getpid()
					if mode == "identity" {
						id = "different-instance"
					}
					if mode == "pid" {
						pid++
					}
					return fmt.Sprintf(`{"ready":true,"instance_id":%q,"process_id":%d,"recovered":false}`, id, pid), 0
				}
				if mode == "old-cli" {
					handler = nil
				}
				input, store := isolatedSSHCommands(t, handler)
				remote = store
				if password {
					input.Password = "fixture-password"
				}
				// Keep a genuine local core alive and prove the remote command never touches it.
				s := testSession(t)
				b := NewBridge(s)
				localInfo := requireReady(t, b)
				cfg, token, err := s.readLocal()
				if err != nil {
					t.Fatal(err)
				}
				localPID := s.launchedPID
				r := b.ConnectSSH(input)
				if r.Problem != nil || r.Data == nil {
					t.Fatalf("enroll: %+v", r)
				}
				selected := *r.Data
				if denied := b.RestartCore(ConfirmStop{InstanceID: localInfo.InstanceID, Approved: true}); denied.Problem == nil || denied.Problem.Code != "identity" || calls.Load() != 0 {
					t.Fatalf("wrong approval reached remote: %+v", denied)
				}
				r = b.RestartCore(ConfirmStop{InstanceID: selected.InstanceID, Approved: true})
				if mode == "success" {
					if r.Problem != nil || r.Data == nil || *r.Data != selected {
						t.Fatalf("target changed: %+v", r)
					}
					// The explicit CLI context reuses exactly the same SSH restart protocol.
					var stdout bytes.Buffer
					args := []string{"--data-dir", s.options.Dir, "--context", input.Name, "restart", "--approve"}
					if password {
						args = append([]string{"--ssh-password-stdin"}, args...)
					}
					err := (cli.App{In: strings.NewReader(input.Password), Out: &stdout, Err: io.Discard}).Run(context.Background(), args)
					var result struct {
						Ready      bool   `json:"ready"`
						InstanceID string `json:"instance_id"`
						ProcessID  int    `json:"process_id"`
					}
					if err != nil || json.Unmarshal(stdout.Bytes(), &result) != nil || !result.Ready || result.InstanceID != selected.InstanceID || result.ProcessID != os.Getpid() {
						t.Fatalf("CLI remote restart: %v %s", err, stdout.String())
					}
				} else {
					want := "unknown"
					if mode == "old-cli" {
						want = "remote_anchor_unsupported"
					}
					if r.Data != nil || r.Problem == nil || r.Problem.Code != want {
						t.Fatalf("unsafe result: %+v", r)
					}
				}
				wantCalls := int32(1)
				if mode == "success" {
					wantCalls = 2
				}
				if mode == "old-cli" {
					wantCalls = 0
				}
				if calls.Load() != wantCalls {
					t.Fatalf("mutation replay: %d", calls.Load())
				}
				cl, info, err := connect(context.Background(), cfg, token)
				if err != nil {
					t.Fatalf("local core changed: %v", err)
				}
				defer cl.Close()
				if info.InstanceID != localInfo.InstanceID || info.ProcessID != localPID {
					t.Fatal("remote restart affected local core")
				}
				status, err := cl.Status(context.Background())
				if err != nil || status["core"] != "ready" {
					t.Fatal("local core no longer ready", err)
				}
			})
		}
	}
}
