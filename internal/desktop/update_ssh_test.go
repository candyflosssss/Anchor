package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/candyflosssss/Anchor/internal/updater"
)

func TestSSHUpdateTargetsAndVerifiedOutcome(t *testing.T) {
	for _, password := range []bool{false, true} {
		for _, mode := range []string{"success", "malformed", "identity", "pid", "failed", "old-cli"} {
			t.Run(fmt.Sprintf("password=%t/%s", password, mode), func(t *testing.T) {
				original := core.Version
				t.Cleanup(func() { core.Version = original })
				var checks, writes atomic.Int32
				var remote *state.Store
				version := "0.1.0-dev.desktop.1000"
				handler := func(args []string) (string, uint32) {
					cfg, _ := remote.Settings()
					port := strings.TrimPrefix(cfg.ControlAddress, "127.0.0.1:")
					if len(args) < 6 || args[0] != "update" {
						t.Errorf("unsafe update args: %q", args)
						return "", 1
					}
					if args[1] == "check" {
						checks.Add(1)
						if strings.Join(args[2:], " ") != "--expected-instance "+cfg.InstanceID+" --control-port "+port {
							t.Errorf("unpinned discovery: %q", args)
						}
						raw, _ := json.Marshal(updater.Check{CurrentVersion: core.Version, Version: version, Channel: "dev", Product: "cli", Available: true, Source: "https://example.invalid/releases", Artifact: &updater.Artifact{File: "anchor", Kind: "binary", Target: "linux-amd64", URL: "https://example.invalid/anchor", SHA256: strings.Repeat("a", 64), Bytes: 100}})
						return string(raw), 0
					}
					writes.Add(1)
					want := "update upgrade --approve --expected-instance " + cfg.InstanceID + " --control-port " + port + " --expected-version " + version + " --expected-sha256 " + strings.Repeat("a", 64)
					if strings.Join(args, " ") != want {
						t.Errorf("unsafe mutation: %q", args)
					}
					if mode == "failed" {
						return "", 1
					}
					if mode == "malformed" {
						return "not-json", 0
					}
					id, pid := cfg.InstanceID, os.Getpid()
					if mode == "identity" {
						id = "another-instance"
					}
					if mode == "pid" {
						pid++
					}
					core.Version = version
					raw, _ := json.Marshal(updater.RuntimeResult{Ready: true, Version: version, InstanceID: id, ProcessID: pid})
					return string(raw), 0
				}
				if mode == "old-cli" {
					handler = nil
				}
				input, store := isolatedSSHCommands(t, handler)
				remote = store
				if password {
					input.Password = "fixture-password"
				}
				s := testSession(t)
				b := NewBridge(s)
				local := requireReady(t, b)
				localPID := s.launchedPID
				result := b.ConnectSSH(input)
				if result.Problem != nil {
					t.Fatal(result)
				}
				id := result.Data.InstanceID
				if denied := b.CheckRemoteUpdate(local.InstanceID); denied.Problem == nil || writes.Load() != 0 || checks.Load() != 0 {
					t.Fatal("wrong instance reached SSH", denied)
				}
				checked := b.CheckRemoteUpdate(id)
				if mode == "old-cli" {
					if checked.Problem == nil || checked.Problem.Code != "remote_anchor_unsupported" {
						t.Fatal(checked)
					}
					return
				}
				if checked.Problem != nil || !checked.Data.Available {
					t.Fatal(checked)
				}
				if denied := b.InstallRemoteUpdate(ConfirmRemoteUpdate{InstanceID: local.InstanceID, Version: version, Approved: true}); denied.Problem == nil || writes.Load() != 0 {
					t.Fatal("wrong confirmation reached SSH", denied)
				}
				updated := b.InstallRemoteUpdate(ConfirmRemoteUpdate{InstanceID: id, Version: version, Approved: true})
				if mode == "success" {
					if updated.Problem != nil || updated.Data.Phase != "applied" || updated.Data.CurrentVersion != version {
						t.Fatal(updated)
					}
				} else {
					if updated.Data != nil || updated.Problem == nil || updated.Problem.Code != "unknown" {
						t.Fatal(updated)
					}
				}
				if writes.Load() != 1 {
					t.Fatal("mutation retried", writes.Load())
				}
				if again := b.InstallRemoteUpdate(ConfirmRemoteUpdate{InstanceID: id, Version: version, Approved: true}); again.Problem == nil || writes.Load() != 1 {
					t.Fatal("confirmation reused", again)
				}
				core.Version = original
				cfg, token, err := s.readLocal()
				if err != nil {
					t.Fatal(err)
				}
				cl, info, err := connect(t.Context(), cfg, token)
				if err != nil {
					t.Fatal(err)
				}
				cl.Close()
				if info.InstanceID != local.InstanceID || info.ProcessID != localPID {
					t.Fatal("SSH update touched local core")
				}
			})
		}
	}
}
