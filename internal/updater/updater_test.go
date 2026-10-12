package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/platform"
)

func testFeed(t *testing.T, product string) (*Service, Config, *Manifest, *string) {
	t.Helper()
	payload := "synthetic update binary"
	hash := sha256.Sum256([]byte(payload))
	m := &Manifest{SchemaVersion: 1, Product: product, Version: "0.1.0-dev.desktop.10", Channel: "dev"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed" {
			_ = json.NewEncoder(w).Encode(m)
		} else {
			_, _ = fmt.Fprint(w, payload)
		}
	}))
	t.Cleanup(server.Close)
	kind, file := "binary", "anchor"
	if product == "desktop" {
		kind, file = "deb", "anchor.deb"
	}
	m.Artifacts = []Artifact{{File: file, Target: "linux-amd64", Kind: kind, URL: server.URL + "/asset", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(payload))}}
	service := New(product, "0.1.0-dev.desktop.9")
	service.Target = "linux-amd64"
	return service, Config{Channel: "dev", ManifestURL: server.URL + "/feed"}, m, &payload
}
func TestCheckDownloadApply(t *testing.T) {
	s, c, _, _ := testFeed(t, "cli")
	dir := t.TempDir()
	check, err := s.Check(context.Background(), c)
	if err != nil || !check.Available {
		t.Fatalf("%+v %v", check, err)
	}
	d, err := s.Download(context.Background(), dir, check)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadDownload(dir); err != nil {
		t.Fatal(err)
	}
	// Idempotent completed downloads do not require a network request.
	s.Client = &http.Client{Transport: failTransport{}}
	if again, err := s.Download(context.Background(), dir, check); err != nil || again.Path != d.Path {
		t.Fatal(again, err)
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(root, "anchor")
	if err := os.WriteFile(exe, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	lock, err := platform.AcquireLock(filepath.Join(dir, "instance.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyBinary(dir, exe); !errors.Is(err, ErrBusy) {
		t.Fatal("did not refuse running core", err)
	}
	lock.Close()
	backup, err := s.ApplyBinary(dir, exe)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := os.ReadFile(backup)
	newBytes, _ := os.ReadFile(exe)
	if string(old) != "old" || string(newBytes) != "synthetic update binary" {
		t.Fatal("replacement/backup mismatch")
	}
	st, _ := os.Stat(exe)
	if st.Mode().Perm() != 0755 {
		t.Fatal("lost executable permissions")
	}
	// A later update overwrites only the last binary backup, not user state.
	if _, err := s.ApplyBinary(dir, exe); err != nil {
		t.Fatal(err)
	}
}

type failTransport struct{}

func (failTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network should not be used")
}
func TestIntegrityAndMalformedMetadata(t *testing.T) {
	for _, change := range []string{"checksum", "size", "path", "kind", "target", "duplicate", "channel", "version", "url"} {
		t.Run(change, func(t *testing.T) {
			s, c, m, payload := testFeed(t, "desktop")
			switch change {
			case "checksum":
				m.Artifacts[0].SHA256 = strings.Repeat("0", 64)
			case "size":
				m.Artifacts[0].Bytes++
			case "path":
				m.Artifacts[0].File = "../danger.deb"
			case "kind":
				m.Artifacts[0].Kind = "binary"
			case "target":
				m.Artifacts[0].Target = "linux-arm64"
			case "duplicate":
				m.Artifacts = append(m.Artifacts, m.Artifacts[0])
			case "channel":
				m.Channel = "stable"
			case "version":
				m.Version = "0.1.0-dev..10"
			case "url":
				m.Artifacts[0].URL = "http://example.com/update.deb"
			}
			check, err := s.Check(context.Background(), c)
			if change == "checksum" || change == "size" {
				if err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				if _, err = s.Download(context.Background(), dir, check); !errors.Is(err, ErrIntegrity) {
					t.Fatal(err)
				}
				files, _ := os.ReadDir(filepath.Join(dir, "updates"))
				for _, f := range files {
					if strings.Contains(f.Name(), "partial") || strings.Contains(f.Name(), "download.json") {
						t.Fatal("unverified artifact retained", f.Name())
					}
				}
			} else if err == nil {
				t.Fatal("invalid manifest accepted", *payload)
			}
		})
	}
}
func TestTamperedReceiptAndSymlink(t *testing.T) {
	s, c, _, _ := testFeed(t, "desktop")
	dir := t.TempDir()
	check, err := s.Check(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Download(context.Background(), dir, check)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.Path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadDownload(dir); !errors.Is(err, ErrIntegrity) {
		t.Fatal(err)
	}
	os.Remove(d.Path)
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("synthetic update binary"), 0600)
	if err := os.Symlink(outside, d.Path); err != nil {
		t.Skip("symlink not supported")
	}
	if _, err := s.ReadDownload(dir); !errors.Is(err, ErrIntegrity) {
		t.Fatal("link accepted", err)
	}
	if _, err := s.Download(context.Background(), dir, check); !errors.Is(err, ErrInvalid) {
		t.Fatal("linked cache overwritten", err)
	}
}
func TestVersionsAndDowngrade(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"0.1.0-dev.desktop.10", "0.1.0-dev.desktop.9", 1}, {"1.0.0-alpha.10", "1.0.0-alpha.2", 1},
		{"1.0.0", "1.0.0-rc.1", 1}, {"1.0.0-beta.1", "1.0.0-alpha.9", 1}, {"1.0.0", "1.0.0", 0},
		{"1.0.0-dev", "1.0.1-alpha.1", -1}, {"1.0.0-1", "1.0.0-a", -1},
	} {
		cmp, err := compareVersions(tc.a, tc.b)
		if err != nil || cmp != tc.want {
			t.Fatal(tc, cmp, err)
		}
	}
	for _, bad := range []string{"01.0.0", "1.0", "1.0.0-dev..1", "1.0.0-alpha.01", "1.0.0+a", "99999999999999.0.0"} {
		if _, err := parseVersion(bad); err == nil {
			t.Fatal("invalid accepted", bad)
		}
	}
	s, c, m, _ := testFeed(t, "desktop")
	m.Version = "0.1.0-dev.desktop.8"
	check, err := s.Check(context.Background(), c)
	if err != nil || check.Available {
		t.Fatal(check, err)
	}
	if _, err := s.Download(context.Background(), t.TempDir(), check); !errors.Is(err, ErrUnavailable) {
		t.Fatal("downgrade accepted", err)
	}
}
func TestConfigAndURLPolicy(t *testing.T) {
	for _, u := range []string{"https://example.com/update.json", "http://127.0.0.1:8787/feed", "http://[::1]:80/feed"} {
		if err := ValidateURL(u); err != nil {
			t.Fatal(u, err)
		}
	}
	for _, u := range []string{"file:///opt/update", "http://localhost/feed", "http://example.com/feed", "https://u:p@example.com/feed", "https://example.com/feed?token=secret", "https://example.com/feed#f"} {
		if err := ValidateURL(u); err == nil {
			t.Fatal("unsafe accepted", u)
		}
	}
	dir := t.TempDir()
	cfg, err := ReadConfig(dir)
	if err != nil || cfg.Channel != "alpha" {
		t.Fatal(cfg, err)
	}
	cfg = Config{Channel: "dev", ManifestURL: "https://example.com/feed.json"}
	if err := Configure(dir, cfg); err != nil {
		t.Fatal(err)
	}
	read, err := ReadConfig(dir)
	if err != nil || read != cfg {
		t.Fatal(read, err)
	}
	if err := Configure(dir, Config{Channel: "nightly"}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
func TestGitHubSelectionAndLegacyManifest(t *testing.T) {
	hash := sha256.Sum256([]byte("binary"))
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases" {
			rows := []map[string]any{}
			for _, v := range []string{"0.1.0-alpha.2", "0.1.0-beta.1", "0.1.0-alpha.10"} {
				rows = append(rows, map[string]any{"tag_name": "v" + v, "draft": false, "assets": []map[string]string{{"name": "release.json", "browser_download_url": base + "/manifest/" + v}, {"name": "anchor", "browser_download_url": base + "/anchor"}}})
			}
			rows = append(rows, map[string]any{"tag_name": "v9.0.0-alpha.1", "draft": true})
			json.NewEncoder(w).Encode(rows)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "version": strings.TrimPrefix(r.URL.Path, "/manifest/"), "channel": "alpha", "standalone_artifacts": []Artifact{{File: "anchor", Target: "linux-amd64", SHA256: hex.EncodeToString(hash[:]), Bytes: 6}}})
	}))
	defer server.Close()
	base = server.URL
	s := New("cli", "0.1.0-alpha.1")
	s.Endpoint = base + "/releases"
	s.Target = "linux-amd64"
	c, err := s.Check(context.Background(), Config{Channel: "alpha"})
	if err != nil || !c.Available || c.Version != "0.1.0-alpha.10" || c.Artifact.Kind != "binary" {
		t.Fatal(c, err)
	}
	c, err = s.Check(context.Background(), Config{Channel: "dev"})
	if err != nil || c.Available || c.Version != "" {
		t.Fatal(c, err)
	}
}
func TestRedirectPolicy(t *testing.T) {
	// Signed HTTPS redirect queries are required by GitHub's release CDN.
	s := New("cli", "0.1.0-alpha.1")
	req, _ := http.NewRequest("GET", "https://cdn.example.com/file?signature=value", nil)
	original, _ := http.NewRequest("GET", "https://example.com", nil)
	if err := s.Client.CheckRedirect(req, []*http.Request{original}); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest("GET", "http://127.0.0.1/file", nil)
	if err := s.Client.CheckRedirect(req, []*http.Request{original}); err == nil {
		t.Fatal("TLS downgrade accepted")
	}
}

func TestDuplicateOtherPlatformRejected(t *testing.T) {
	s, c, m, _ := testFeed(t, "desktop")
	other := m.Artifacts[0]
	other.Target, other.Kind, other.File = "windows-amd64", "nsis", "setup.exe"
	m.Artifacts = append(m.Artifacts, other, other)
	if _, err := s.Check(context.Background(), c); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate non-selected platform accepted", err)
	}
}

// Verify the helper is a private independent copy and failed launches clean up;
// the callback deliberately never executes the helper or any native installer.
func TestInstallerHelperCopyAndCleanup(t *testing.T) {
	dir := t.TempDir()
	if err := platform.SecureDir(filepath.Join(dir, "updates")); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "host.exe")
	if err := os.WriteFile(executable, []byte("isolated host copy"), 0700); err != nil {
		t.Fatal(err)
	}
	var helper string
	approved := InstallApproval{Version: "0.1.0-dev.desktop.10", SHA256: strings.Repeat("a", 64)}
	rejected := errors.New("synthetic launch failure")
	if err := LaunchInstallerHelper(dir, executable, approved, func(path string, args []string) error {
		helper = path
		if filepath.Dir(path) != filepath.Join(dir, "updates") || len(args) != 8 || args[4] != "--install-version" || args[5] != approved.Version || args[6] != "--install-sha256" || args[7] != approved.SHA256 || args[0] != "--data-dir" || args[1] != dir || args[2] != "--finish-update" {
			t.Fatal("unsafe helper args", path, args)
		}
		content, err := os.ReadFile(path)
		if err != nil || string(content) != "isolated host copy" {
			t.Fatal("helper copy mismatch", err)
		}
		return rejected
	}); !errors.Is(err, rejected) {
		t.Fatal(err)
	}
	if _, err := os.Lstat(helper); !os.IsNotExist(err) {
		t.Fatal("failed helper retained", err)
	}
	if err := LaunchInstallerHelper(dir, executable, approved, func(path string, args []string) error {
		helper = path
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(helper); err != nil {
		t.Fatal("successful helper missing", err)
	}
}

func TestInstallerApprovalBoundToVersionAndBytes(t *testing.T) {
	s, cfg, manifest, payload := testFeed(t, "desktop")
	c, err := s.Check(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	d, err := s.Download(context.Background(), dir, c)
	if err != nil {
		t.Fatal(err)
	}
	approved := InstallApproval{Version: d.Check.Version, SHA256: d.Check.Artifact.SHA256}
	if _, err := s.readApprovedDownload(dir, approved); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []InstallApproval{
		{Version: "0.1.0-dev.desktop.11", SHA256: approved.SHA256},
		{Version: approved.Version, SHA256: strings.Repeat("a", 64)},
		{},
	} {
		if _, err := s.readApprovedDownload(dir, changed); !errors.Is(err, ErrInvalid) {
			t.Fatal("unapproved installer accepted", changed, err)
		}
	}
	// Replace the receipt via a second legitimate download while the old window
	// is closing. The helper must not install that newly discovered candidate.
	manifest.Version = "0.1.0-dev.desktop.11"
	newer, err := s.Check(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Download(context.Background(), dir, newer); err != nil {
		t.Fatal(err)
	}
	if _, err := s.readApprovedDownload(dir, approved); !errors.Is(err, ErrInvalid) {
		t.Fatal("new receipt bypassed version confirmation", err)
	}
	approved = InstallApproval{Version: newer.Version, SHA256: newer.Artifact.SHA256}
	// Even replacing an artifact under the same version cannot swap its bytes.
	*payload = "different synthetic installer bytes"
	hash := sha256.Sum256([]byte(*payload))
	manifest.Artifacts[0].SHA256 = hex.EncodeToString(hash[:])
	manifest.Artifacts[0].Bytes = int64(len(*payload))
	replaced, err := s.Check(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Download(context.Background(), dir, replaced); err != nil {
		t.Fatal(err)
	}
	if _, err := s.readApprovedDownload(dir, approved); !errors.Is(err, ErrInvalid) {
		t.Fatal("same-version receipt bypassed hash confirmation", err)
	}
}

func TestVersionChannelDefaultsPreserveSavedConfig(t *testing.T) {
	for _, tc := range []struct{ version, channel string }{
		{"0.1.0-dev.desktop.18", "dev"},
		{"0.1.0-alpha.6", "alpha"},
		{"0.1.0-beta.1", "beta"},
		{"0.1.0", "stable"},
		{"dev", "alpha"},
	} {
		t.Run(tc.version, func(t *testing.T) {
			dir := t.TempDir()
			cfg, err := ReadConfigForVersion(dir, tc.version)
			if err != nil || cfg.Channel != tc.channel || cfg.ManifestURL != "" {
				t.Fatal(cfg, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "updates", "config.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("reading defaults must not persist configuration", err)
			}
			saved := Config{Channel: "rc", ManifestURL: "https://example.com/custom.json"}
			if err := Configure(dir, saved); err != nil {
				t.Fatal(err)
			}
			if cfg, err := ReadConfigForVersion(dir, tc.version); err != nil || cfg != saved {
				t.Fatal("saved channel and source must survive upgrade", cfg, err)
			}
			if err := os.WriteFile(filepath.Join(dir, "updates", "config.json"), []byte(`{"channel":"nightly"}`), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadConfigForVersion(dir, tc.version); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid config must not silently fall back", err)
			}
		})
	}
}
