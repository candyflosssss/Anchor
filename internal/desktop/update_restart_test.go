package desktop

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/candyflosssss/Anchor/internal/updater"
)

// The old provider is deliberately a dead route, as in the upgrade regression.
// Only synthetic home/config/cache and a loopback mock upstream are used.
func restartCodexHome(t *testing.T) (string, []byte) {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".codex")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", root)
	raw, err := os.ReadFile("../state/testdata/codex-models.json")
	if err != nil {
		t.Fatal(err)
	}
	var cache map[string]any
	if err = json.Unmarshal(raw, &cache); err != nil {
		t.Fatal(err)
	}
	cache["client_version"] = "0.159.3"
	raw, err = json.Marshal(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "models_cache.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	original := []byte("model = \"old\"\nmodel_provider = \"custom\"\n[model_providers.custom]\nname = \"old-local-proxy\"\nbase_url = \"http://127.0.0.1:15722/v1\"\n")
	if err = os.WriteFile(filepath.Join(root, "config.toml"), original, 0600); err != nil {
		t.Fatal(err)
	}
	return root, original
}

func enrollRestartBinding(t *testing.T, s *Session, b *Bridge, root string) (func(), func()) {
	t.Helper()
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" || r.Header.Get("Authorization") != "Bearer fixture-upstream" {
			t.Error("wrong upstream route or credential")
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"response-fixture","object":"response","status":"completed","output":[]}`)
	}))
	t.Cleanup(upstream.Close)
	st, err := state.Open(s.options.Dir, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	secret := "fixture-upstream"
	if _, err = st.PutProvider(state.Provider{ID: "p", BaseURL: upstream.URL, Protocol: "responses", Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, 0, &secret); err != nil {
		t.Fatal(err)
	}
	if _, err = st.PutModel(state.Model{ID: "public", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture-source"}}}, 0); err != nil {
		t.Fatal(err)
	}
	workspace := b.AgentWorkspace()
	if workspace.Problem != nil || workspace.Data == nil || len(workspace.Data.Targets) != 1 {
		t.Fatal("discovery failed", workspace)
	}
	enrolled := b.EnrollAgent(EnrollAgent{TargetID: workspace.Data.Targets[0].ID, Model: "public", Approved: true})
	if enrolled.Problem != nil || enrolled.Data == nil || len(enrolled.Data.Bindings) != 1 || enrolled.Data.Bindings[0].Status != "applied" {
		t.Fatal("enrollment failed", enrolled)
	}
	id := enrolled.Data.Bindings[0].ID
	token, err := st.BindingToken(id)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(root, "config.toml"), filepath.Join(root, "anchor_"+id+".models.json"), filepath.Join(s.options.Dir, "recovery", id+".json")}
	read := func(path string) []byte {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	snapshots := make([][]byte, len(paths))
	for i, p := range paths {
		snapshots[i] = read(p)
	}
	cfg, err := st.Settings()
	if err != nil {
		t.Fatal(err)
	}
	address := "http://" + cfg.GatewayAddress + "/v1/responses"
	unchanged := func() {
		t.Helper()
		for i, p := range paths {
			if !bytes.Equal(snapshots[i], read(p)) {
				t.Fatalf("handoff rewrote %s", filepath.Base(p))
			}
		}
		now, err := st.BindingToken(id)
		if err != nil || now != token {
			t.Fatal("credential changed", err)
		}
	}
	probe := func() {
		t.Helper()
		req, err := http.NewRequest("POST", address, strings.NewReader(`{"model":"public","input":"fixture"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		before := requests.Load()
		cl := &http.Client{Timeout: s.options.StartTimeout}
		defer cl.CloseIdleConnections()
		resp, err := cl.Do(req)
		if err != nil {
			t.Fatal("saved client route did not reconnect", err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		if resp.StatusCode != 200 || requests.Load() != before+1 {
			t.Fatalf("actual proxy request failed: status=%d", resp.StatusCode)
		}
	}
	return unchanged, probe
}

func TestUpdateRefusesOldCoreWithoutShutdown(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux fixture")
	}
	// A held lifecycle lock and authenticated old-core fixture must not cause the
	// desktop to fall back to ordinary shutdown, even if graceful_stop exists.
	var writes atomic.Int32
	var instance string
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/control/v1/instance" {
			json.NewEncoder(w).Encode(map[string]any{"instance_id": instance, "api_version": 1, "capabilities": []string{"graceful_stop"}})
			return
		}
		writes.Add(1)
		t.Error("old core received a mutation")
	}))
	defer web.Close()
	dir := t.TempDir()
	st, err := state.Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg, err := st.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860")
	if err != nil {
		t.Fatal(err)
	}
	instance = cfg.InstanceID
	lock, err := platform.AcquireLock(filepath.Join(dir, "instance.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	s := NewSession(Options{Dir: dir, Quit: func() { t.Error("unsupported update quit desktop") }})
	defer s.Close()
	b := NewBridge(s)
	version := downloadRestartUpdate(t, b, dir)
	r := b.InstallUpdate(ConfirmUpdate{Version: version, Approved: true})
	if r.Problem == nil || r.Problem.Code != "unsupported" || writes.Load() != 0 || s.updating {
		t.Fatal("old core was not protected", r)
	}
}

func downloadRestartUpdate(t *testing.T, b *Bridge, dir string) string {
	t.Helper()
	payload := []byte("synthetic package, never executed")
	hash := sha256.Sum256(payload)
	var base string
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/feed" {
			json.NewEncoder(w).Encode(updater.Manifest{SchemaVersion: 1, Product: "desktop", Version: "0.1.0-dev.desktop.1000", Channel: "dev", Artifacts: []updater.Artifact{{File: "anchor.deb", Target: "linux-amd64", Kind: "deb", URL: base + "/package", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(payload))}}})
		} else {
			w.Write(payload)
		}
	}))
	t.Cleanup(web.Close)
	base = web.URL
	if err := updater.Configure(dir, updater.Config{Channel: "dev", ManifestURL: base + "/feed"}); err != nil {
		t.Fatal(err)
	}
	c := b.CheckUpdate("dev")
	if c.Problem != nil || !c.Data.Available {
		t.Fatal(c)
	}
	d := b.DownloadUpdate(c.Data.LatestVersion)
	if d.Problem != nil {
		t.Fatal(d)
	}
	return c.Data.LatestVersion
}
