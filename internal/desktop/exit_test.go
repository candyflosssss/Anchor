package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestTrayExitStopsStartedAndReusedLocalCore(t *testing.T) {
	for _, reused := range []bool{false, true} {
		t.Run(map[bool]string{false: "started", true: "reused"}[reused], func(t *testing.T) {
			s := testSession(t)
			info := requireReady(t, NewBridge(s))
			cfg, token, err := s.readLocal()
			if err != nil {
				t.Fatal(err)
			}
			if reused {
				s = NewSession(s.options)
				defer s.Close()
				if got := requireReady(t, NewBridge(s)); got.Source != "reused" {
					t.Fatal("did not reuse local core", got)
				}
			}
			quits := 0
			s.options.Quit = func() { quits++ }
			out := s.ExitWithLocalCore(func(id string) bool {
				if id != info.InstanceID {
					t.Errorf("wrong confirmation target: %q", id)
				}
				return true
			})
			if out.Problem != nil || out.Data == nil || !out.Data.Accepted || quits != 1 || s.cfg != nil || s.ctx.Err() == nil {
				t.Fatalf("stop-and-exit failed: %#v quits=%d", out, quits)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if cl, _, err := connect(ctx, cfg, token); err == nil {
				cl.Close()
				t.Fatal("quit ran while the local core was still reachable")
			}
			if got := NewBridge(s).Start(); got.Problem == nil {
				t.Fatal("queued Start replaced a core after exit was accepted")
			}
		})
	}
}

func TestTrayExitRestoresManagedConfig(t *testing.T) {
	root, original := restartCodexHome(t)
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	unchanged, probe := enrollRestartBinding(t, s, b, root)
	unchanged()
	probe()
	s.options.Quit = func() {}
	out := s.ExitWithLocalCore(func(string) bool { return true })
	if out.Problem != nil || out.Data == nil || !out.Data.Accepted {
		t.Fatal(out)
	}
	raw, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil || !bytes.Equal(raw, original) {
		t.Fatal("ordinary shutdown did not restore the synthetic Agent config", err)
	}
}

func TestTrayExitWithoutCoreNeverInitializesOrStarts(t *testing.T) {
	for _, missing := range []bool{false, true} {
		for _, approved := range []bool{false, true} {
			name := map[bool]string{false: "existing", true: "missing"}[missing] + map[bool]string{false: "/cancel", true: "/approve"}[approved]
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				if missing {
					dir = filepath.Join(dir, "not-created")
				}
				quits, confirms := 0, 0
				s := NewSession(Options{Dir: dir, Quit: func() { quits++ }})
				defer s.Close()
				out := s.ExitWithLocalCore(func(id string) bool {
					confirms++
					if id != "" {
						t.Error("invented a running core")
					}
					return approved
				})
				if out.Problem != nil || out.Data == nil || out.Data.Accepted != approved || confirms != 1 || (quits == 1) != approved {
					t.Fatalf("unexpected no-core exit: %#v confirms=%d quits=%d", out, confirms, quits)
				}
				if _, err := os.Stat(filepath.Join(dir, "state.db")); !os.IsNotExist(err) {
					t.Fatal("exit created state", err)
				}
				if missing {
					if _, err := os.Stat(dir); !os.IsNotExist(err) {
						t.Fatal("exit created the data directory", err)
					}
				}
			})
		}
	}
}

func TestTrayExitRejectsInitializationDuringNoCoreConfirmation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new-state")
	s := NewSession(Options{Dir: dir, Quit: func() { t.Error("quit without rechecking new state") }})
	defer s.Close()
	out := s.ExitWithLocalCore(func(id string) bool {
		if id != "" {
			t.Error("invented a running core")
		}
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		return true
	})
	if out.Problem == nil || out.Problem.Code != "conflict" || out.Data != nil {
		t.Fatal("new state was not rejected", out)
	}
}

type trayExitFixture struct {
	session  *Session
	id       atomic.Value
	writes   atomic.Int32
	quits    atomic.Int32
	release  func()
	instance string
}

func newTrayExitFixture(t *testing.T, capabilities []string, stop func(*trayExitFixture, http.ResponseWriter)) *trayExitFixture {
	t.Helper()
	f := &trayExitFixture{}
	var token string
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token || r.Header.Get("X-Anchor-Instance") != f.instance {
			t.Error("missing local authorization or instance binding")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/control/v1/instance":
			json.NewEncoder(w).Encode(map[string]any{"instance_id": f.id.Load(), "api_version": 1, "capabilities": capabilities})
		case "/control/v1/shutdown":
			f.writes.Add(1)
			var in map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in["for_restart"] {
				t.Error("tray exit must use ordinary shutdown, not restart handoff", err)
			}
			stop(f, w)
		default:
			t.Errorf("unexpected tray request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(web.Close)
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
	token, err = st.OwnerToken()
	if err != nil {
		t.Fatal(err)
	}
	f.instance = cfg.InstanceID
	f.id.Store(cfg.InstanceID)
	lock, err := platform.AcquireLock(filepath.Join(dir, "instance.lock"))
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	f.release = func() { once.Do(func() { lock.Close() }) }
	t.Cleanup(f.release)
	f.session = NewSession(Options{Dir: dir, Quit: func() { f.quits.Add(1) }})
	f.session.cfg = &cfg
	t.Cleanup(f.session.Close)
	return f
}

func TestTrayExitCancelDoesNotStop(t *testing.T) {
	f := newTrayExitFixture(t, []string{"graceful_stop"}, func(*trayExitFixture, http.ResponseWriter) { t.Error("cancelled stop") })
	out := f.session.ExitWithLocalCore(func(id string) bool { return false })
	if out.Problem != nil || out.Data == nil || out.Data.Accepted || f.writes.Load() != 0 || f.quits.Load() != 0 || f.session.ctx.Err() != nil {
		t.Fatal("cancel changed the session", out)
	}
}

func TestTrayExitFailuresKeepHostOpenWithoutRetry(t *testing.T) {
	for _, mode := range []string{"unsupported", "identity-before", "identity-after", "unauthorized", "rejected", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			caps := []string{"graceful_stop"}
			if mode == "unsupported" {
				caps = nil
			}
			f := newTrayExitFixture(t, caps, func(_ *trayExitFixture, w http.ResponseWriter) {
				switch mode {
				case "unauthorized":
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte(`{"code":"unauthorized"}`))
				case "rejected":
					w.Write([]byte(`{"accepted":false}`))
				default:
					w.Write([]byte(`{"accepted":`))
				}
			})
			if mode == "identity-before" {
				f.id.Store("different-instance")
			}
			confirms := 0
			out := f.session.ExitWithLocalCore(func(id string) bool {
				confirms++
				if mode == "identity-after" {
					f.id.Store("different-instance")
				}
				return true
			})
			wantWrites, wantConfirms := int32(1), 1
			if mode == "unsupported" || mode == "identity-before" {
				wantWrites, wantConfirms = 0, 0
			} else if mode == "identity-after" {
				wantWrites = 0
			}
			if out.Problem == nil || out.Data != nil || f.writes.Load() != wantWrites || confirms != wantConfirms || f.quits.Load() != 0 || f.session.ctx.Err() != nil {
				t.Fatalf("failure was not contained: %#v writes=%d confirms=%d quits=%d", out, f.writes.Load(), confirms, f.quits.Load())
			}
		})
	}
}

func TestTrayExitWaitsForLifetimeLockAfterAcceptance(t *testing.T) {
	accepted := make(chan struct{})
	f := newTrayExitFixture(t, []string{"graceful_stop"}, func(_ *trayExitFixture, w http.ResponseWriter) {
		w.Write([]byte(`{"accepted":true}`))
		close(accepted)
	})
	done := make(chan StopResult, 1)
	go func() { done <- f.session.ExitWithLocalCore(func(string) bool { return true }) }()
	select {
	case <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("stop request did not arrive")
	}
	select {
	case <-done:
		t.Fatal("acceptance was incorrectly treated as completed shutdown")
	case <-time.After(150 * time.Millisecond):
	}
	if f.quits.Load() != 0 {
		t.Fatal("quit before lock release")
	}
	f.release()
	select {
	case out := <-done:
		if out.Problem != nil || out.Data == nil || !out.Data.Accepted || f.writes.Load() != 1 || f.quits.Load() != 1 {
			t.Fatal("did not finish after lock release", out)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("exit did not finish")
	}
}

func TestTrayExitTimeoutNeverForcesOrRetries(t *testing.T) {
	f := newTrayExitFixture(t, []string{"graceful_stop"}, func(_ *trayExitFixture, w http.ResponseWriter) { w.Write([]byte(`{"accepted":true}`)) })
	ctx, cancel := context.WithTimeout(f.session.ctx, time.Second)
	defer cancel()
	f.session.ctx = ctx
	out := f.session.ExitWithLocalCore(func(string) bool { return true })
	if out.Problem == nil || out.Problem.Code != "conflict" || f.writes.Load() != 1 || f.quits.Load() != 0 {
		t.Fatal("timeout was not contained", out)
	}
	lock, err := platform.AcquireLock(filepath.Join(f.session.options.Dir, "instance.lock"))
	if err == nil {
		lock.Close()
		t.Fatal("timeout force-released the running core's lock")
	}
}

func TestTrayExitCannotInterruptUpdateOrStartup(t *testing.T) {
	for _, mode := range []string{"update", "startup", "no-quit", "closed"} {
		t.Run(mode, func(t *testing.T) {
			s := NewSession(Options{Dir: t.TempDir(), Quit: func() { t.Error("quit during " + mode) }})
			defer s.Close()
			switch mode {
			case "update":
				s.updating = true
			case "startup":
				lock, err := platform.AcquireLock(filepath.Join(s.options.Dir, "desktop-start.lock"))
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
			case "no-quit":
				s.options.Quit = nil
			case "closed":
				s.Close()
			}
			out := s.ExitWithLocalCore(func(string) bool { t.Error("unexpected confirmation"); return true })
			if out.Problem == nil || out.Data != nil {
				t.Fatal("busy exit should fail closed", out)
			}
		})
	}
}

func TestTrayExitWhileRemoteActiveStopsOnlyLocal(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	local := requireReady(t, b)
	input, remoteStore := isolatedSSH(t)
	input.TrustNewHost = true
	remote := b.ConnectSSH(input)
	if remote.Problem != nil || remote.Data == nil || remote.Data.Source != "remote" {
		t.Fatal(remote)
	}
	quits := 0
	s.options.Quit = func() { quits++ }
	out := s.ExitWithLocalCore(func(id string) bool {
		if id != local.InstanceID || id == remote.Data.InstanceID {
			t.Error("tray selected the remote instance")
		}
		return true
	})
	if out.Problem != nil || out.Data == nil || !out.Data.Accepted || quits != 1 {
		t.Fatal(out)
	}
	cfg, err := remoteStore.Settings()
	if err != nil {
		t.Fatal(err)
	}
	token, err := remoteStore.OwnerToken()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cl, info, err := connect(ctx, cfg, token)
	if err != nil {
		t.Fatal("tray exit stopped the remote core", err)
	}
	defer cl.Close()
	if info.InstanceID != remote.Data.InstanceID {
		t.Fatal("remote identity changed")
	}
}
