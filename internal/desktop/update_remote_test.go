package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/candyflosssss/Anchor/internal/updater"
)

// Installing this desktop must stop only its local core, even while the UI is
// operating a remote instance. Failed installer handoff keeps the remote usable.
func TestUpdateWhileRemoteActiveStopsOnlyLocal(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	input, _ := isolatedSSH(t)
	input.TrustNewHost = true
	remote := b.ConnectSSH(input)
	if remote.Problem != nil {
		t.Fatal(remote)
	}
	payload := []byte("fixture package, never executed")
	hash := sha256.Sum256(payload)
	var base string
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed" {
			json.NewEncoder(w).Encode(updater.Manifest{SchemaVersion: 1, Product: "desktop", Version: "0.1.0-dev.desktop.1000", Channel: "dev", Artifacts: []updater.Artifact{{File: "anchor.deb", Target: "linux-amd64", Kind: "deb", URL: base + "/package", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(payload))}}})
		} else {
			w.Write(payload)
		}
	}))
	defer web.Close()
	base = web.URL
	if err := updater.Configure(s.options.Dir, updater.Config{Channel: "dev", ManifestURL: base + "/feed"}); err != nil {
		t.Fatal(err)
	}
	c := b.CheckUpdate("dev")
	if c.Problem != nil || !c.Data.Available {
		t.Fatal(c)
	}
	if d := b.DownloadUpdate(c.Data.LatestVersion); d.Problem != nil {
		t.Fatal(d)
	}
	quit := false
	s.options.Quit = func() { quit = true }
	s.options.Executable = "missing-update-helper-executable"
	r := b.InstallUpdate(ConfirmUpdate{Version: c.Data.LatestVersion, Approved: true})
	if r.Problem == nil || quit || s.updating {
		t.Fatal("failed installer handoff claimed success", r)
	}
	waitStopped(t, s.options.Dir)
	status := b.Status()
	if status.Problem != nil || status.Data.InstanceID != remote.Data.InstanceID || status.Data.Transport != "ssh" {
		t.Fatal("local update interrupted remote instance", status)
	}
}
