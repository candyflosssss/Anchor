package desktop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/updater"
)

func TestRuntimeUpdateOneHandoffPreservesManagedClient(t *testing.T) {
	for _, mode := range []string{"GUI", "CLI-desktop", "CLI-core"} {
		t.Run(mode, func(t *testing.T) { runtimeUpdateHandoff(t, mode) })
	}
}
func runtimeUpdateHandoff(t *testing.T, mode string) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux subprocess fixture")
	}
	root, _ := restartCodexHome(t)
	s := testSession(t)
	b := NewBridge(s)
	before := requireReady(t, b)
	cfg, token, err := s.readLocal()
	if err != nil {
		t.Fatal(err)
	}
	cl, old, err := connect(context.Background(), cfg, token)
	if err != nil {
		t.Fatal(err)
	}
	cl.Close()
	unchanged, probe := enrollRestartBinding(t, s, b, root)
	probe()
	executable, _ := os.Executable()
	payload, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	version := "0.1.0-dev.desktop.1000"
	t.Setenv("ANCHOR_TEST_RUNTIME_VERSION", version)
	product, kind, subcommand := "desktop", "runtime", "desktop"
	if mode == "CLI-core" {
		product, kind, subcommand = "cli", "binary", "upgrade"
	}
	var base string
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed" {
			json.NewEncoder(w).Encode(updater.Manifest{SchemaVersion: 1, Product: product, Version: version, Channel: "dev", Artifacts: []updater.Artifact{{File: "runtime", Kind: kind, Target: "linux-amd64", URL: base + "/runtime", SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(payload))}}})
			return
		}
		w.Write(payload)
	}))
	defer web.Close()
	base = web.URL
	if err := updater.Configure(s.options.Dir, updater.Config{Channel: "dev", ManifestURL: base + "/feed"}); err != nil {
		t.Fatal(err)
	}
	if mode == "GUI" {
		checked := b.CheckUpdate("dev")
		if checked.Problem != nil {
			t.Fatal(checked)
		}
		if d := b.DownloadUpdate(version); d.Problem != nil {
			t.Fatal(d)
		}
	}
	// Helper creation is exercised, but no GUI/helper process is spawned by this
	// headless fixture. The replacement core is a genuine detached subprocess.
	oldLauncher := runtimeHelperLauncher
	defer func() { runtimeHelperLauncher = oldLauncher }()
	helpers := 0
	runtimeHelperLauncher = func(path string, args []string) error {
		helpers++
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
		os.Remove(path)
		return nil
	}
	quit := false
	s.options.Quit = func() { quit = true }
	if mode != "GUI" {
		var stdout bytes.Buffer
		err := (cli.App{In: strings.NewReader(""), Out: &stdout, Err: io.Discard}).Run(context.Background(), []string{"--data-dir", s.options.Dir, "update", subcommand, "--approve"})
		var result updater.RuntimeResult
		if err != nil || json.Unmarshal(stdout.Bytes(), &result) != nil || !result.Ready || result.Version != version {
			t.Fatalf("CLI update: %v %s", err, stdout.String())
		}
	} else {
		// Failure to prepare the window handoff must leave the old core running.
		original := s.options.Executable
		s.options.Executable = "missing-runtime-helper"
		if failed := b.InstallUpdate(ConfirmUpdate{Version: version, Approved: true}); failed.Problem == nil {
			t.Fatal("missing helper accepted")
		}
		cl, still, err := connect(context.Background(), cfg, token)
		if err != nil {
			t.Fatal(err)
		}
		cl.Close()
		if still.ProcessID != old.ProcessID {
			t.Fatal("helper preflight interrupted old service")
		}
		s.options.Executable = original
		r := b.InstallUpdate(ConfirmUpdate{Version: version, Approved: true})
		if r.Problem != nil || r.Data == nil || !quit || helpers != 1 {
			t.Fatalf("handoff: %+v", r)
		}
	}
	unchanged()
	probe()
	cl, now, err := connect(context.Background(), cfg, token)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if now.InstanceID != before.InstanceID || now.ProcessID == old.ProcessID || now.Version != version {
		t.Fatalf("replacement mismatch: %+v", now)
	}
	active, err := updater.ActiveRuntime(s.options.Dir, product, core.Version)
	if err != nil || active.Version != version {
		t.Fatal(active, err)
	}
	// Opening the updated GUI reuses this core instead of restarting it again.
	fresh := NewSession(Options{Dir: s.options.Dir, Executable: active.Path})
	defer fresh.Close()
	if status := NewBridge(fresh).Start(); status.Problem != nil || status.Data.InstanceID != before.InstanceID {
		t.Fatal(status)
	}
	cl, again, err := connect(context.Background(), cfg, token)
	if err != nil {
		t.Fatal(err)
	}
	cl.Close()
	// The original installed desktop launcher routes the next invocation to the
	// active executable, with no config rewrite or core restart.
	if mode != "CLI-core" {
		if err := updater.New("cli", core.Version).ActivateRuntime(s.options.Dir, updater.Runtime{Version: active.Version, Artifact: updater.Artifact{File: "anchor", Kind: "binary", Target: active.Artifact.Target, URL: active.Artifact.URL, SHA256: active.Artifact.SHA256, Bytes: active.Artifact.Bytes}, Path: active.Path}); err == nil {
			t.Fatal("foreign product path accepted")
		}
	}
	// Desktop forwarding is exercised through the same host-owned API as main.
	forwardMode := "--desktop-test-forward"
	if mode == "CLI-core" {
		forwardMode = "--desktop-test-cli"
	}
	cmd := exec.Command(executable, forwardMode, "--data-dir", s.options.Dir, "version")
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("launcher forwarding: %v %s", err, raw)
	}
	var forwarded struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &forwarded) != nil || forwarded.Version != version {
		t.Fatalf("launcher returned old version: %s", raw)
	}
	if again.ProcessID != now.ProcessID {
		t.Fatal("GUI reopened with a second service interruption")
	}
}
