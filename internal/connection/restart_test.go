package connection

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestRestartLocalFailClosed(t *testing.T) {
	for _, mode := range []string{"old-core", "identity", "unauthorized", "unknown", "timeout", "launch-failed", "start-conflict", "bootstrap-conflict", "ready-timeout"} {
		t.Run(mode, func(t *testing.T) {
			var identity, token string
			var writes, launches atomic.Int32
			web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+token {
					t.Error("restart lost owner authentication")
				}
				if mode == "unauthorized" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if r.URL.Path == "/control/v1/instance" {
					id := identity
					if mode == "identity" {
						id = "another-instance"
					}
					caps := []string{"graceful_stop", "restart_handoff"}
					if mode == "old-core" {
						caps = caps[:1]
					}
					json.NewEncoder(w).Encode(map[string]any{"instance_id": id, "api_version": 1, "capabilities": caps})
					return
				}
				if r.URL.Path == "/control/v1/status" && mode == "ready-timeout" {
					json.NewEncoder(w).Encode(map[string]string{"core": "starting"})
					return
				}
				if r.URL.Path != "/control/v1/shutdown" || r.Method != "POST" {
					t.Error("unexpected request")
					return
				}
				writes.Add(1)
				var body map[string]bool
				json.NewDecoder(r.Body).Decode(&body)
				if !body["for_restart"] || len(body) != 1 {
					t.Error("ordinary stop fallback")
				}
				// An accepted response without the preservation acknowledgement is unknown.
				json.NewEncoder(w).Encode(map[string]bool{"accepted": true, "bindings_preserved": mode != "unknown"})
			}))
			defer web.Close()
			dir := t.TempDir()
			st, err := state.Open(dir, true)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := st.Initialize(web.Listener.Addr().String(), "127.0.0.1:17860")
			if err != nil {
				t.Fatal(err)
			}
			identity = cfg.InstanceID
			token, err = st.OwnerToken()
			st.Close()
			if err != nil {
				t.Fatal(err)
			}
			lockName := ""
			switch mode {
			case "timeout":
				lockName = "instance.lock"
			case "start-conflict":
				lockName = "desktop-start.lock"
			case "bootstrap-conflict":
				lockName = "remote-start.lock"
			}
			if lockName != "" {
				lock, err := platform.AcquireLock(filepath.Join(dir, lockName))
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
			defer cancel()
			_, err = RestartLocal(ctx, dir, identity, func() (int, <-chan error, error) {
				launches.Add(1)
				if mode == "ready-timeout" {
					return 100, nil, nil
				}
				return 0, nil, errors.New("fixture launch failure")
			})
			if err == nil {
				t.Fatal("unsafe restart accepted")
			}
			wantWrites, wantLaunches := int32(0), int32(0)
			switch mode {
			case "unknown", "timeout":
				wantWrites = 1
			case "launch-failed", "ready-timeout":
				wantWrites = 1
				wantLaunches = 1
			}
			if writes.Load() != wantWrites || launches.Load() != wantLaunches {
				t.Fatalf("writes=%d launches=%d", writes.Load(), launches.Load())
			}
			if mode == "unknown" {
				var req *client.RequestError
				if !errors.As(err, &req) || !req.Unknown {
					t.Fatal("lost unknown outcome", err)
				}
			}
			if mode == "identity" && !errors.Is(err, client.ErrIdentity) {
				t.Fatal("lost identity failure", err)
			}
			// Coordination locks are always released, including timeout and unknown.
			if mode != "start-conflict" {
				lock, e := platform.AcquireLock(filepath.Join(dir, "desktop-start.lock"))
				if e != nil {
					t.Fatal("leaked startup lock", e)
				}
				lock.Close()
			}
		})
	}
}

func TestRestartLocalDoesNotInitializeMissingState(t *testing.T) {
	dir := t.TempDir()
	launches := 0
	_, err := RestartLocal(context.Background(), dir, "", func() (int, <-chan error, error) { launches++; return 0, nil, nil })
	if err == nil || launches != 0 {
		t.Fatal("initialized/launched unapproved state")
	}
	if _, err := os.Stat(filepath.Join(dir, "state.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("created database", err)
	}
}

func TestRestartLocalPinsRejectBeforeContact(t *testing.T) {
	var contacts atomic.Int32
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacts.Add(1); w.WriteHeader(500) }))
	defer web.Close()
	st, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg, err := st.Initialize(web.Listener.Addr().String(), "127.0.0.1:17860")
	if err != nil {
		t.Fatal(err)
	}
	_, portText, _ := net.SplitHostPort(cfg.ControlAddress)
	port, _ := strconv.Atoi(portText)
	for _, tc := range []struct {
		id   string
		port int
		code string
	}{
		{"wrong-instance", port, "identity"},
		{cfg.InstanceID, port%65535 + 1, "remote_control_port"},
	} {
		launches := 0
		_, err := RestartLocalPinned(context.Background(), st.Dir, tc.id, tc.port, func() (int, <-chan error, error) { launches++; return 0, nil, nil })
		if err == nil || contacts.Load() != 0 || launches != 0 {
			t.Fatalf("pin mismatch mutated state: %v", err)
		}
		if tc.code == "identity" && !errors.Is(err, client.ErrIdentity) {
			t.Fatal(err)
		}
		if tc.code != "identity" {
			var e *Error
			if !errors.As(err, &e) || e.Code != tc.code {
				t.Fatal(err)
			}
		}
	}
}
