package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/connection"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestRestartCorePreservesBindingAndRecoversOffline(t *testing.T) {
	root, _ := restartCodexHome(t)
	s := testSession(t)
	b := NewBridge(s)
	info := requireReady(t, b)
	unchanged, probe := enrollRestartBinding(t, s, b, root)
	probe()
	oldPID := s.launchedPID
	r := b.RestartCore(ConfirmStop{InstanceID: info.InstanceID, Approved: true})
	if r.Problem != nil || r.Data == nil || r.Data.InstanceID != info.InstanceID || r.Data.Phase != "ready" || r.Data.Source != "started" || oldPID == s.launchedPID {
		t.Fatalf("restart: %+v", r)
	}
	unchanged()
	probe()
	// Stop just the isolated core with handoff. Status does not relaunch it.
	cfg, token, err := s.readLocal()
	if err != nil {
		t.Fatal(err)
	}
	cl, _, err := connect(context.Background(), cfg, token)
	if err != nil {
		t.Fatal(err)
	}
	if err := cl.StopForRestart(context.Background()); err != nil {
		t.Fatal(err)
	}
	cl.Close()
	waitStopped(t, s.options.Dir)
	if status := b.Status(); status.Problem == nil {
		t.Fatal("offline status pretended to be ready")
	}
	unchanged()
	r = b.RestartCore(ConfirmStop{InstanceID: info.InstanceID, Approved: true})
	if r.Problem != nil || r.Data == nil || r.Data.Phase != "ready" {
		t.Fatalf("recovery: %+v", r)
	}
	unchanged()
	probe()
}

func TestRestartCoreGuards(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	info := requireReady(t, b)
	for _, in := range []ConfirmStop{{}, {InstanceID: info.InstanceID}, {Approved: true}} {
		if r := b.RestartCore(in); r.Problem == nil || r.Problem.Code != "invalid" {
			t.Fatalf("approval: %+v", r)
		}
	}
	if r := b.RestartCore(ConfirmStop{InstanceID: "wrong-instance", Approved: true}); r.Problem == nil || r.Problem.Code != "identity" {
		t.Fatalf("identity: %+v", r)
	}
	s.mu.Lock()
	concurrent := b.RestartCore(ConfirmStop{InstanceID: info.InstanceID, Approved: true})
	s.mu.Unlock()
	if concurrent.Problem == nil || concurrent.Problem.Code != "conflict" {
		t.Fatalf("concurrent: %+v", concurrent)
	}
	s.updating = true
	if r := b.RestartCore(ConfirmStop{InstanceID: info.InstanceID, Approved: true}); r.Problem == nil || r.Problem.Code != "conflict" {
		t.Fatalf("update: %+v", r)
	}
	s.updating = false
	s.remote = &connection.Live{Profile: state.Context{Name: "remote"}}
	if r := b.RestartCore(ConfirmStop{InstanceID: info.InstanceID, Approved: true}); r.Problem == nil || r.Problem.Code != "identity" {
		t.Fatalf("remote: %+v", r)
	}
	s.remote = nil
	st, err := state.Open(s.options.Dir, false)
	if err != nil {
		t.Fatal(err)
	}
	// A persisted remote target is not allowed to fall back to this host either.
	if err := st.PutContext(state.Context{Name: "remote", Transport: "ssh", SSHTarget: "fixture@192.0.2.1", SSHPort: 22, RemoteExecutable: "anchor", RemoteControlPort: 17861, InstanceID: "remote-fixture", AccessID: "fixture-access", CredentialRef: "ssh-manage"}); err != nil {
		t.Fatal(err)
	}
	if err := st.SelectDesktop("remote"); err != nil {
		t.Fatal(err)
	}
	st.Close()
	if r := b.RestartCore(ConfirmStop{InstanceID: info.InstanceID, Approved: true}); r.Problem == nil || r.Problem.Code != "unsupported" {
		t.Fatalf("saved remote: %+v", r)
	}
	if status := b.Status(); status.Problem != nil {
		t.Fatal("guards stopped core", status)
	}
}

func TestCLIRestartAndOfflineRecovery(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	info := requireReady(t, b)
	oldPID := s.launchedPID
	var stdout bytes.Buffer
	app := cli.App{Out: &stdout, Err: io.Discard}
	if err := app.Run(context.Background(), []string{"--data-dir", s.options.Dir, "restart", "--approve", "--timeout", "10s"}); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Ready      bool   `json:"ready"`
		InstanceID string `json:"instance_id"`
		ProcessID  int    `json:"process_id"`
		Recovered  bool   `json:"recovered"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Ready || result.InstanceID != info.InstanceID || result.ProcessID == oldPID || result.Recovered {
		t.Fatalf("CLI output: %s", stdout.String())
	}
	if r := b.StopCore(ConfirmStop{InstanceID: info.InstanceID, Approved: true}); r.Problem != nil {
		t.Fatal(r)
	}
	waitStopped(t, s.options.Dir)
	stdout.Reset()
	if err := app.Run(context.Background(), []string{"--data-dir", s.options.Dir, "restart", "--approve"}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Ready || !result.Recovered || result.InstanceID != info.InstanceID {
		t.Fatalf("offline output: %s", stdout.String())
	}
}

func TestSharedCoreRestartContract(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/core-restart.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Contract      string
		Request       ConfirmStop
		Success       SessionResult
		RemoteSuccess SessionResult `json:"remote_success"`
		Errors        map[string]Problem
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.Contract != Contract || !f.Request.Approved || f.Success.Data == nil || f.Success.Data.InstanceID != f.Request.InstanceID || f.Success.Data.Transport != "local" || f.Success.Problem != nil {
		t.Fatal("restart contract drift")
	}
	if f.RemoteSuccess.Data == nil || f.RemoteSuccess.Data.Transport != "ssh" || f.RemoteSuccess.Data.Source != "remote" {
		t.Fatal("remote restart fixture drift")
	}
	if *problem(&connection.Error{Code: "startup_failed"}, true) != f.Errors["startup_failed"] {
		t.Fatal("restart problem drift")
	}
}
