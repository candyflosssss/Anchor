package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/updater"
)

func TestUpdateContractFixture(t *testing.T) {
	b, err := os.ReadFile("../../desktop/contract/updates.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Contract     string        `json:"contract"`
		Downloaded   UpdateInfo    `json:"downloaded"`
		Confirmation ConfirmUpdate `json:"confirmation"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if f.Contract != Contract || f.Downloaded.LatestVersion != f.Confirmation.Version || !f.Confirmation.Approved || f.Downloaded.Phase != "downloaded" {
		t.Fatal("update fixture drift")
	}
	for _, tc := range []struct {
		err  error
		code string
	}{{updater.ErrIntegrity, "integrity"}, {updater.ErrInvalid, "invalid"}, {updater.ErrBusy, "conflict"}, {updater.ErrUnavailable, "unsupported"}, {os.ErrPermission, "permission"}, {errors.New("raw credential"), "read_error"}} {
		if p := updateProblem(tc.err); p.Code != tc.code {
			t.Fatal(p)
		}
	}
}
func TestUpdateStatusAndApproval(t *testing.T) {
	dir := t.TempDir()
	s := NewSession(Options{Dir: dir})
	defer s.Close()
	b := NewBridge(s)
	status := b.UpdateStatus()
	if status.Problem != nil || status.Data.Phase != "idle" || status.Data.Channel != "dev" {
		t.Fatal(status)
	}
	if r := b.DownloadUpdate("untrusted"); r.Problem == nil || r.Problem.Code != "invalid" {
		t.Fatal(r)
	}
	if r := b.InstallUpdate(ConfirmUpdate{Version: "untrusted", Approved: false}); r.Problem == nil || r.Problem.Code != "invalid" {
		t.Fatal(r)
	}
	if r := b.CheckUpdate("nightly"); r.Problem == nil || r.Problem.Code != "invalid" {
		t.Fatal(r)
	}
	// A completed installer handoff cannot automatically start another core.
	s.updating = true
	if r := b.Start(); r.Problem == nil || r.Problem.Code != "conflict" {
		t.Fatal(r)
	}
}

func TestUpdateBridgeRealDownload(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux fixture")
	}
	payload := []byte("isolated installer, never executed")
	hash := sha256.Sum256(payload)
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed" {
			json.NewEncoder(w).Encode(updater.Manifest{SchemaVersion: 1, Product: "desktop", Version: "0.1.0-dev.desktop.10", Channel: "dev", Artifacts: []updater.Artifact{{File: "anchor.deb", Target: "linux-amd64", Kind: "deb", URL: base + "/package", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(payload))}}})
		} else {
			w.Write(payload)
		}
	}))
	defer server.Close()
	base = server.URL
	dir := t.TempDir()
	if err := updater.Configure(dir, updater.Config{Channel: "dev", ManifestURL: base + "/feed"}); err != nil {
		t.Fatal(err)
	}
	s := NewSession(Options{Dir: dir})
	defer s.Close()
	bridge := NewBridge(s)
	c := bridge.CheckUpdate("dev")
	if c.Problem != nil || !c.Data.Available {
		t.Fatal(c)
	}
	if r := bridge.DownloadUpdate("different"); r.Problem == nil {
		t.Fatal("unconfirmed version accepted")
	}
	d := bridge.DownloadUpdate(c.Data.LatestVersion)
	if d.Problem != nil || d.Data.Phase != "downloaded" {
		t.Fatal(d)
	}
	status := bridge.UpdateStatus()
	if status.Problem != nil || status.Data.DownloadPath != d.Data.DownloadPath {
		t.Fatal(status)
	}
	for _, cfg := range []updater.Config{
		{Channel: "beta", ManifestURL: base + "/feed"},
		{Channel: "dev", ManifestURL: "https://example.invalid/another-feed"},
	} {
		if err := updater.Configure(dir, cfg); err != nil {
			t.Fatal(err)
		}
		r := bridge.UpdateStatus()
		if r.Problem != nil || r.Data.Phase != "idle" || r.Data.Channel != cfg.Channel || r.Data.Source != cfg.ManifestURL {
			t.Fatal("stale source/channel receipt reused", r)
		}
	}
	if err := updater.Configure(dir, updater.Config{Channel: "dev", ManifestURL: base + "/feed"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.Data.DownloadPath, []byte("tamper"), 0600); err != nil {
		t.Fatal(err)
	}
	if r := bridge.UpdateStatus(); r.Problem == nil || r.Problem.Code != "integrity" {
		t.Fatal("tampering accepted", r)
	}
}

// Exercise the real authenticated shutdown without launching a native package
// installer or altering an installed executable. A missing helper executable
// deliberately makes the handoff fail after the isolated core has stopped.
func TestUpdateInstallShutdownAndFailedHandoff(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux fixture")
	}
	payload := []byte("synthetic package, never executed")
	hash := sha256.Sum256(payload)
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed" {
			json.NewEncoder(w).Encode(updater.Manifest{SchemaVersion: 1, Product: "desktop", Version: "0.1.0-dev.desktop.10", Channel: "dev", Artifacts: []updater.Artifact{{File: "anchor.deb", Target: "linux-amd64", Kind: "deb", URL: base + "/package", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(payload))}}})
		} else {
			w.Write(payload)
		}
	}))
	defer server.Close()
	base = server.URL
	s := testSession(t)
	bridge := NewBridge(s)
	requireReady(t, bridge)
	quit := false
	s.options.Quit = func() { quit = true }
	if err := updater.Configure(s.options.Dir, updater.Config{Channel: "dev", ManifestURL: base + "/feed"}); err != nil {
		t.Fatal(err)
	}
	c := bridge.CheckUpdate("dev")
	if c.Problem != nil || !c.Data.Available {
		t.Fatal(c)
	}
	d := bridge.DownloadUpdate(c.Data.LatestVersion)
	if d.Problem != nil {
		t.Fatal(d)
	}
	if r := bridge.InstallUpdate(ConfirmUpdate{Version: c.Data.LatestVersion}); r.Problem == nil {
		t.Fatal("unapproved installation accepted")
	}
	if r := bridge.InstallUpdate(ConfirmUpdate{Version: "wrong", Approved: true}); r.Problem == nil {
		t.Fatal("wrong version accepted")
	}
	if err := os.WriteFile(d.Data.DownloadPath, []byte("tamper"), 0600); err != nil {
		t.Fatal(err)
	}
	if r := bridge.InstallUpdate(ConfirmUpdate{Version: c.Data.LatestVersion, Approved: true}); r.Problem == nil || r.Problem.Code != "integrity" {
		t.Fatal(r)
	}
	if r := bridge.Status(); r.Problem != nil {
		t.Fatal("invalid installation stopped the core", r)
	}
	if err := os.WriteFile(d.Data.DownloadPath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	s.options.Executable = "missing-update-helper-executable"
	r := bridge.InstallUpdate(ConfirmUpdate{Version: c.Data.LatestVersion, Approved: true})
	if r.Problem == nil || r.Data != nil || quit || s.updating {
		t.Fatal("failed handoff claimed success or quit", r)
	}
	waitStopped(t, s.options.Dir)
	if status := bridge.Status(); status.Problem == nil {
		t.Fatal("core did not exit normally")
	}
}

func TestDesktopDefaultsToInstalledChannel(t *testing.T) {
	old := core.Version
	core.Version = "0.1.0-dev.desktop.18"
	t.Cleanup(func() { core.Version = old })
	dir := t.TempDir()
	s := NewSession(Options{Dir: dir})
	defer s.Close()
	b := NewBridge(s)
	status := b.UpdateStatus()
	if status.Problem != nil || status.Data == nil || status.Data.Channel != "dev" {
		t.Fatal(status)
	}
	if err := updater.Configure(dir, updater.Config{Channel: "alpha"}); err != nil {
		t.Fatal(err)
	}
	status = b.UpdateStatus()
	if status.Problem != nil || status.Data == nil || status.Data.Channel != "alpha" {
		t.Fatal("explicit channel must keep precedence", status)
	}
}
