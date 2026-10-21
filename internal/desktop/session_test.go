package desktop

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/candyflosssss/Anchor/internal/updater"
)

func TestMain(m *testing.M) {
	if version := os.Getenv("ANCHOR_TEST_RUNTIME_VERSION"); version != "" && (filepath.Base(os.Args[0]) == "anchor-desktop" || filepath.Base(os.Args[0]) == "anchor") {
		core.Version = version
	}
	if len(os.Args) == 2 && os.Args[1] == "--runtime-version" {
		json.NewEncoder(os.Stdout).Encode(map[string]string{"version": core.Version})
		os.Exit(0)
	}

	if len(os.Args) > 1 && os.Args[1] == "--desktop-test-forward" {
		dir, err := CLIDataDir(os.Args[2:])
		if err != nil {
			os.Exit(1)
		}
		forwarded, code, err := updater.ForwardRuntime(context.Background(), dir, "desktop", core.Version, os.Args[1:])
		if err != nil {
			os.Exit(1)
		}
		if forwarded {
			os.Exit(code)
		}
		os.Exit(cli.Main(context.Background(), os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "--desktop-test-cli" {
		os.Exit(cli.Main(context.Background(), os.Args[2:]))
	}
	if IsAgentTokenInvocation(os.Args[1:]) {
		os.Exit(cli.Main(context.Background(), os.Args[1:]))
	}
	if len(os.Args) == 5 && os.Args[1] == "--desktop-test-host" {
		exe, _ := os.Executable()
		s := NewSession(Options{Dir: os.Args[2], Executable: exe, ControlAddress: os.Args[3], GatewayAddress: os.Args[4], StartTimeout: 5 * time.Second})
		result := NewBridge(s).Start()
		s.Close()
		_ = json.NewEncoder(os.Stdout).Encode(result)
		if result.Problem != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if len(os.Args) == 4 && (os.Args[3] == "--core" || os.Args[3] == "serve") {
		err := (cli.App{Out: io.Discard, Err: io.Discard, In: strings.NewReader("")}).Run(context.Background(), []string{"--data-dir", os.Args[2], "serve"})
		if err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if filepath.Base(os.Args[0]) == "anchor" {
		os.Exit(cli.Main(context.Background(), os.Args[1:]))
	}
	os.Exit(m.Run())
}
func addresses(t *testing.T) (string, string) {
	t.Helper()
	a, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	return a.Addr().String(), b.Addr().String()
}
func testSession(t *testing.T) *Session {
	t.Helper()
	control, gateway := addresses(t)
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	s := NewSession(Options{Dir: t.TempDir(), Executable: exe, ControlAddress: control, GatewayAddress: gateway, StartTimeout: 5 * time.Second})
	t.Cleanup(func() {
		cfg, token, e := s.readLocal()
		if e == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			cl, _, e := connect(ctx, cfg, token)
			if e == nil {
				_ = cl.Stop(ctx)
				cl.Close()
			}
		}
		// No test-owned child may outlive its isolated data directory.
		if _, err := os.Stat(filepath.Join(s.options.Dir, "instance.lock")); err == nil {
			waitStopped(t, s.options.Dir)
		}
		s.Close()
	})
	return s
}
func requireReady(t *testing.T, b *Bridge) *SessionInfo {
	t.Helper()
	r := b.Start()
	if r.Problem != nil || r.Data == nil || r.Data.Phase != "ready" {
		t.Fatalf("start: %+v", r)
	}
	return r.Data
}
func TestRealCoreLifecycleAndProviders(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	info := requireReady(t, b)
	if info.Source != "started" || !info.CanStop || info.Version != core.Version {
		t.Fatalf("info: %+v", info)
	}
	if r := b.Providers(); r.Problem != nil || r.Data == nil || len(r.Data) != 0 {
		t.Fatalf("empty list: %+v", r)
	}
	secret := "fixture-secret-not-real"
	input := SaveProvider{Provider: Provider{ID: "供应商测试", BaseURLs: []string{"https://api.example.com/v1"}, Enabled: true, TimeoutSeconds: 120, MaxConcurrent: 4}, Secret: &secret}
	created := b.SaveProvider(input)
	if created.Problem != nil || created.Data == nil || !created.Data.HasSecret || created.Data.Revision != 1 {
		t.Fatalf("create: %+v", created)
	}
	wire, _ := json.Marshal(created)
	if strings.Contains(string(wire), secret) {
		t.Fatal("secret leaked")
	}
	if r := b.SaveProvider(input); r.Problem == nil || r.Problem.Code != "conflict" {
		t.Fatalf("stale create: %+v", r)
	}
	input.Provider = *created.Data
	input.ExpectedRevision = 1
	input.Secret = nil
	input.Provider.Enabled = false
	updated := b.SaveProvider(input)
	if updated.Problem != nil || !updated.Data.HasSecret || updated.Data.Enabled {
		t.Fatalf("keep: %+v", updated)
	}
	empty := ""
	input.Provider = *updated.Data
	input.ExpectedRevision = 2
	input.Secret = &empty
	cleared := b.SaveProvider(input)
	if cleared.Problem != nil || cleared.Data.HasSecret {
		t.Fatalf("clear: %+v", cleared)
	}
	input.Provider.Revision = MaxSafeRevision
	input.ExpectedRevision = MaxSafeRevision
	if r := b.SaveProvider(input); r.Problem == nil || r.Problem.Code != "invalid" {
		t.Fatal("unsafe revision accepted")
	}
	// Window close only cancels host calls. A fresh host must reconnect to persisted data.
	s.Close()
	next := NewSession(s.options)
	defer next.Close()
	other := NewBridge(next)
	again := requireReady(t, other)
	if again.InstanceID != info.InstanceID || again.Source != "reused" {
		t.Fatal("did not reuse")
	}
	if r := other.Providers(); r.Problem != nil || len(r.Data) != 1 || r.Data[0].Revision != 3 {
		t.Fatalf("persisted: %+v", r)
	}
	if r := other.StopCore(ConfirmStop{InstanceID: info.InstanceID}); r.Problem == nil {
		t.Fatal("unconfirmed shutdown accepted")
	}
	if r := other.StopCore(ConfirmStop{InstanceID: "wrong", Approved: true}); r.Problem == nil {
		t.Fatal("wrong instance accepted")
	}
	if r := other.StopCore(ConfirmStop{InstanceID: info.InstanceID, Approved: true}); r.Problem != nil || r.Data == nil || !r.Data.Accepted {
		t.Fatalf("stop: %+v", r)
	}
}
func TestConcurrentHostsShareOneCore(t *testing.T) {
	s := testSession(t)
	other := NewSession(s.options)
	defer other.Close()
	var a, b SessionResult
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); a = NewBridge(s).Start() }()
	go func() { defer wg.Done(); b = NewBridge(other).Start() }()
	wg.Wait()
	if a.Problem != nil || b.Problem != nil || a.Data == nil || b.Data == nil || a.Data.InstanceID != b.Data.InstanceID {
		t.Fatalf("concurrent: %+v %+v", a, b)
	}
	if a.Data.Source == b.Data.Source {
		t.Fatal("expected one starter and one reuser")
	}
}
func TestIdentityFailureDoesNotLaunchOrOverwrite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"instance_id":"wrong","api_version":1}`)
	}))
	defer srv.Close()
	_, gateway := addresses(t)
	s := NewSession(Options{Dir: t.TempDir(), Executable: "missing-executable", ControlAddress: strings.TrimPrefix(srv.URL, "http://"), GatewayAddress: gateway, StartTimeout: time.Second})
	defer s.Close()
	r := NewBridge(s).Start()
	if r.Problem == nil || r.Problem.Code != "identity" {
		t.Fatalf("identity: %+v", r)
	}
	st, e := state.Open(s.options.Dir, false)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	cfg, _ := st.Settings()
	if cfg.ControlAddress != s.options.ControlAddress {
		t.Fatal("overwritten address")
	}
}
func TestClosedHostCannotStart(t *testing.T) {
	s := testSession(t)
	s.Close()
	r := NewBridge(s).Start()
	if r.Problem == nil || r.Problem.Code != "closed" {
		t.Fatal("closed host started")
	}
}
func TestTypeScriptContractDrift(t *testing.T) {
	b, e := os.ReadFile("../../desktop/frontend/src/desktop/contract.generated.ts")
	if e != nil {
		t.Fatal(e)
	}
	if string(b) != TypeScript() {
		t.Fatal("regenerate TypeScript contract")
	}
}

func waitStopped(t *testing.T, dir string) {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		lock, err := platform.AcquireLock(filepath.Join(dir, "instance.lock"))
		if err == nil {
			lock.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("test-owned core did not release its instance lock")
}
func TestCoreSurvivesHostProcessExit(t *testing.T) {
	s := testSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.options.Executable, "--desktop-test-host", s.options.Dir, s.options.ControlAddress, s.options.GatewayAddress)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("host process: %v", err)
	}
	var first SessionResult
	if json.Unmarshal(out, &first) != nil || first.Problem != nil || first.Data == nil {
		t.Fatal("invalid host output")
	}
	// The helper has really exited, not just canceled a simulated window context.
	b := NewBridge(s)
	info := requireReady(t, b)
	if info.Source != "reused" || info.InstanceID != first.Data.InstanceID {
		t.Fatal("core died with host")
	}
	if r := b.StopCore(ConfirmStop{InstanceID: info.InstanceID, Approved: true}); r.Problem != nil {
		t.Fatalf("stop: %+v", r)
	}
	waitStopped(t, s.options.Dir)
}
func TestStartupRejectsUnauthorizedAndOccupiedGateway(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
		defer web.Close()
		_, gateway := addresses(t)
		s := NewSession(Options{Dir: t.TempDir(), Executable: "must-not-launch", ControlAddress: strings.TrimPrefix(web.URL, "http://"), GatewayAddress: gateway, StartTimeout: time.Second})
		defer s.Close()
		if r := NewBridge(s).Start(); r.Problem == nil || r.Problem.Code != "unauthorized" {
			t.Fatalf("authorization: %+v", r)
		}
	})
	t.Run("gateway-port-occupied", func(t *testing.T) {
		s := testSession(t)
		occupied, err := net.Listen("tcp", s.options.GatewayAddress)
		if err != nil {
			t.Fatal(err)
		}
		defer occupied.Close()
		if r := NewBridge(s).Start(); r.Problem == nil || r.Problem.Code != "startup_failed" {
			t.Fatalf("occupied gateway: %+v", r)
		}
		// Failed startup must not take over or close the unrelated listener.
		c, err := net.DialTimeout("tcp", occupied.Addr().String(), time.Second)
		if err != nil {
			t.Fatal("unrelated listener was stopped")
		}
		c.Close()
	})
}

func TestStartupDoesNotRecreateMissingOwnerCredential(t *testing.T) {
	s := testSession(t)
	st, err := state.Open(s.options.Dir, true)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := st.Initialize(s.options.ControlAddress, s.options.GatewayAddress)
	st.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.options.Dir, "owner.token")); err != nil {
		t.Fatal(err)
	}
	if r := NewBridge(s).Start(); r.Problem == nil || r.Problem.Code != "startup_failed" {
		t.Fatalf("missing credential: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(s.options.Dir, "owner.token")); !os.IsNotExist(err) {
		t.Fatal("missing credential silently replaced")
	}
	st, err = state.Open(s.options.Dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	after, err := st.Settings()
	if err != nil || after != cfg {
		t.Fatal("existing settings overwritten")
	}
}

func TestRealCoreProviderDirectory(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/v1/sub2api/billing" {
			w.WriteHeader(404)
			return
		}
		if r.URL.Path != "/v1/models" || r.Method != "GET" || (r.Header.Get("Authorization") != "Bearer synthetic-catalog-secret" && r.Header.Get("Authorization") != "Bearer synthetic-catalog-second") {
			t.Error("unexpected upstream request")
		}
		io.WriteString(w, `{"data":[{"id":"fixture-model"}]}`)
	}))
	defer upstream.Close()
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	secret := "synthetic-catalog-secret"
	created := b.SaveProvider(SaveProvider{Provider: Provider{ID: "目录供应商", BaseURLs: []string{upstream.URL + "/v1"}, Enabled: false, TimeoutSeconds: 2, MaxConcurrent: 1}, Secret: &secret})
	if created.Problem != nil || created.Data == nil {
		t.Fatal(created)
	}
	in := DiscoverProviderModels{ProviderID: created.Data.ID, ExpectedRevision: created.Data.Revision}
	result := b.DiscoverProviderModels(in)
	if result.Problem != nil || result.Data == nil || result.Data.HTTPStatus != 200 || result.Data.Failure != "" || len(result.Data.Models) != 1 || result.Data.Models[0] != "fixture-model" {
		t.Fatal(result)
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), secret) {
		t.Fatal("secret leaked")
	}
	// Saved configuration and its revision remain unchanged after a real bridge call.
	rows := b.Providers()
	if rows.Problem != nil || len(rows.Data) != 1 || !reflect.DeepEqual(rows.Data[0], *created.Data) {
		t.Fatal(rows)
	}
	in.ExpectedRevision++
	conflict := b.DiscoverProviderModels(in)
	if conflict.Problem == nil || conflict.Problem.Code != "conflict" || calls.Load() != 2 {
		t.Fatal("revision conflict or retry", conflict, calls.Load())
	}
	in.ExpectedRevision = MaxSafeRevision + 1
	if invalid := b.DiscoverProviderModels(in); invalid.Problem == nil || invalid.Problem.Code != "invalid" {
		t.Fatal(invalid)
	}
	added := b.ProviderAction(Action{ProviderID: in.ProviderID, ExpectedRevision: created.Data.Revision, Action: "key-put", Target: "second", Secret: "synthetic-catalog-second", Protocol: "responses", Enabled: true})
	if added.Problem != nil {
		t.Fatal(added)
	}
	in.ExpectedRevision = added.Data.Revision
	merged := b.DiscoverProviderModels(in)
	if merged.Problem != nil || merged.Data == nil || merged.Data.KeyID != "" || len(merged.Data.Models) != 1 || len(merged.Data.Entries) != 1 || len(merged.Data.Entries[0].KeyIDs) != 2 || len(merged.Data.Keys) != 2 || !merged.Data.Keys[1].Enabled || calls.Load() != 6 {
		t.Fatal("bridge catalog associations lost", merged)
	}
	disabled := b.ProviderAction(Action{ProviderID: in.ProviderID, ExpectedRevision: in.ExpectedRevision, Action: "key-put", Target: "second", Enabled: false})
	if disabled.Problem != nil || disabled.Data == nil {
		t.Fatal(disabled)
	}
	in.ExpectedRevision = disabled.Data.Revision
	filtered := b.DiscoverProviderModels(in)
	if filtered.Problem != nil || filtered.Data == nil || len(filtered.Data.Keys) != 1 || !reflect.DeepEqual(filtered.Data.Entries[0].KeyIDs, []string{"default"}) || calls.Load() != 8 {
		t.Fatal("bridge queried disabled key", filtered)
	}
	in.KeyID = "second"
	skipped := b.DiscoverProviderModels(in)
	if skipped.Problem != nil || skipped.Data == nil || skipped.Data.Failure != "no_enabled_keys" || len(skipped.Data.Keys) != 0 || calls.Load() != 8 {
		t.Fatal("bridge did not preserve disabled key result", skipped)
	}
}

func TestRealBridgeKeyProtocolSettings(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	secret := "synthetic-protocol"
	created := b.SaveProvider(SaveProvider{Provider: Provider{ID: "protocol", BaseURLs: []string{"http://127.0.0.1:1"}, Enabled: false, TimeoutSeconds: 2, MaxConcurrent: 1}, Secret: &secret})
	if created.Problem != nil {
		t.Fatal(created)
	}
	written := b.ProviderAction(Action{ProviderID: "protocol", ExpectedRevision: 1, Action: "key-put", Target: "default", Protocol: "anthropic", Enabled: true})
	if written.Problem != nil || written.Data.Keys[0].Protocol != "anthropic" || written.Data.Revision != 2 {
		t.Fatal(written)
	}
	written = b.ProviderAction(Action{ProviderID: "protocol", ExpectedRevision: 2, Action: "key-model-protocol", Target: "default", Model: "actual/model", Protocol: "chat-completions"})
	if written.Problem != nil || written.Data.Revision != 3 || len(written.Data.Keys[0].ModelProtocols) != 1 {
		t.Fatal(written)
	}
	conflict := b.ProviderAction(Action{ProviderID: "protocol", ExpectedRevision: 2, Action: "key-model-protocol", Target: "default", Model: "actual/model", Protocol: "inherit"})
	if conflict.Problem == nil || conflict.Problem.Code != "conflict" {
		t.Fatal(conflict)
	}
	read := b.ProviderWorkspace(WorkspaceQuery{ProviderID: "protocol"})
	if read.Problem != nil || read.Data.Revision != 3 || read.Data.Keys[0].ModelProtocols[0].Protocol != "chat-completions" {
		t.Fatal(read)
	}
	raw, _ := json.Marshal(read)
	if strings.Contains(string(raw), secret) {
		t.Fatal("credential leaked")
	}
	written = b.ProviderAction(Action{ProviderID: "protocol", ExpectedRevision: 3, Action: "key-model-protocol", Target: "default", Model: "actual/model", Protocol: "inherit"})
	if written.Problem != nil || len(written.Data.Keys[0].ModelProtocols) != 0 || written.Data.Keys[0].Protocol != "anthropic" {
		t.Fatal(written)
	}
}

func TestRealLogBridgeQuerySettingsAndExport(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	if v := b.Start(); v.Problem != nil {
		t.Fatal(v.Problem)
	}
	v := b.Logs(LogQuery{Limit: 50})
	if v.Problem != nil || v.Data == nil || v.Data.Requests == nil {
		t.Fatal(v)
	}
	cfg := b.LogSettings()
	if cfg.Problem != nil || cfg.Data == nil {
		t.Fatal(cfg)
	}
	cfg.Data.Days = 14
	saved := b.ConfigureLogs(*cfg.Data)
	if saved.Problem != nil || saved.Data == nil || saved.Data.Days != 14 {
		t.Fatal(saved)
	}
	path := filepath.Join(t.TempDir(), "logs.jsonl")
	s.options.SaveLogFile = func() (string, error) { return path, nil }
	exported := b.ExportLogs(LogQuery{Kind: "system"})
	if exported.Problem != nil || exported.Data == nil || !exported.Data.Accepted {
		t.Fatal(exported)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	parts, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.partial"))
	if err != nil || len(parts) != 0 {
		t.Fatal(parts, err)
	}
	s.options.SaveLogFile = func() (string, error) { return "", nil }
	if v := b.ExportLogs(LogQuery{}); v.Problem != nil || v.Data.Accepted {
		t.Fatal(v)
	}
}
