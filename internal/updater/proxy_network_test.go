package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// This is a real TLS/CONNECT test: direct dialing is rejected. Check, manifest,
// download, and a signed cross-host redirect all have to traverse the proxy.
func TestUpdateCheckDownloadThroughProxy(t *testing.T) {
	for _, source := range []string{"system", "ALL_PROXY"} {
		t.Run(source, func(t *testing.T) {
			payload := "isolated installer"
			hash := sha256.Sum256([]byte(payload))
			var mu sync.Mutex
			var hosts []string
			origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api":
					json.NewEncoder(w).Encode([]map[string]any{{"tag_name": "v0.1.0-dev.desktop.20", "assets": []map[string]string{{"name": "update-desktop.json", "browser_download_url": "https://github.com/manifest"}, {"name": "setup.deb", "browser_download_url": "https://github.com/setup.deb"}}}})
				case "/manifest":
					json.NewEncoder(w).Encode(Manifest{SchemaVersion: 1, Product: "desktop", Version: "0.1.0-dev.desktop.20", Channel: "dev", Artifacts: []Artifact{{File: "setup.deb", Target: "linux-amd64", Kind: "deb", URL: "https://github.com/setup.deb", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(payload))}}})
				case "/setup.deb":
					http.Redirect(w, r, "https://release-assets.githubusercontent.com/payload?signature=synthetic", http.StatusFound)
				case "/payload":
					io.WriteString(w, payload)
				default:
					http.NotFound(w, r)
				}
			}))
			defer origin.Close()
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "CONNECT" {
					t.Error("expected CONNECT")
					http.Error(w, "CONNECT required", 400)
					return
				}
				mu.Lock()
				hosts = append(hosts, r.Host)
				mu.Unlock()
				upstream, err := net.Dial("tcp", origin.Listener.Addr().String())
				if err != nil {
					t.Error(err)
					return
				}
				defer upstream.Close()
				conn, buf, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				fmt.Fprint(buf, "HTTP/1.1 200 Connection Established\r\n\r\n")
				buf.Flush()
				done := make(chan struct{})
				go func() { io.Copy(upstream, buf); upstream.Close(); close(done) }()
				io.Copy(conn, upstream)
				conn.Close()
				<-done
			}))
			defer proxy.Close()
			oldSys, oldEnv := systemProxy, lookupEnv
			defer func() { systemProxy, lookupEnv = oldSys, oldEnv }()
			lookupEnv = func(k string) string {
				if source == "ALL_PROXY" && k == "ALL_PROXY" {
					return proxy.URL
				}
				return ""
			}
			systemProxy = func(_ context.Context, _ *url.URL) (*url.URL, error) {
				if source != "system" {
					t.Error("ALL_PROXY must win")
				}
				return url.Parse(proxy.URL)
			}
			s := New("desktop", "0.1.0-dev.desktop.19")
			s.Target = "linux-amd64"
			s.Endpoint = "https://api.github.com/api"
			tr := s.Client.Transport.(*http.Transport)
			defer tr.CloseIdleConnections()
			tr.TLSClientConfig = origin.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			tr.TLSClientConfig.ServerName = "example.com" // httptest certificate; hosts are isolated routing fixtures
			tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				if address != proxy.Listener.Addr().String() {
					return nil, fmt.Errorf("direct dialing forbidden")
				}
				return (&net.Dialer{}).DialContext(ctx, network, address)
			}
			check, err := s.Check(context.Background(), Config{Channel: "dev"})
			if err != nil || !check.Available {
				t.Fatal(check, err)
			}
			d, err := s.Download(context.Background(), t.TempDir(), check)
			if err != nil {
				t.Fatal(err)
			}
			if d.Check.Version != "0.1.0-dev.desktop.20" {
				t.Fatal(d)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, host := range []string{"api.github.com:443", "github.com:443", "release-assets.githubusercontent.com:443"} {
				if !strings.Contains(strings.Join(hosts, ","), host) {
					t.Fatal("phase did not traverse proxy", host, hosts)
				}
			}
		})
	}
}
