package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/platform"
)

func TestHostInstanceConcurrentLaunches(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var shows atomic.Int32
	var wg sync.WaitGroup
	results := make(chan *HostInstance, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			host, err := AcquireHost(ctx, dir, func() bool { shows.Add(1); return true })
			if err != nil {
				t.Errorf("launch failed: %v", err)
			}
			results <- host
		}()
	}
	wg.Wait()
	close(results)
	owners := 0
	for host := range results {
		if host != nil {
			owners++
			t.Cleanup(host.Close)
		}
	}
	if owners != 1 || shows.Load() != 7 {
		t.Fatalf("owners=%d activations=%d, want one GUI and seven activations", owners, shows.Load())
	}
}

func TestHostInstanceStartupAndExit(t *testing.T) {
	dir := t.TempDir()
	var window WindowLifecycle
	host, err := AcquireHost(t.Context(), dir, window.Show)
	if err != nil || host == nil {
		t.Fatalf("first launch: %v", err)
	}
	t.Cleanup(host.Close)
	raw, err := os.ReadFile(filepath.Join(dir, "desktop-host.json"))
	if err != nil {
		t.Fatal(err)
	}
	var endpoint hostEndpoint
	if err := json.Unmarshal(raw, &endpoint); err != nil || endpoint.ProcessID != uint32(os.Getpid()) {
		t.Fatal("activation metadata must identify the GUI host process", err, endpoint.ProcessID)
	}
	second, err := AcquireHost(t.Context(), dir, window.Show)
	if err != nil || second != nil {
		t.Fatalf("launch before window ready: host=%v err=%v", second != nil, err)
	}
	shows := 0
	window.Ready(func() { shows++ })
	if shows != 1 {
		t.Fatalf("startup activation lost: shows=%d", shows)
	}
	window.RequestExit()
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	if duplicate, err := AcquireHost(ctx, dir, window.Show); err == nil || duplicate != nil {
		t.Fatal("exiting host must not accept activation or permit a second GUI")
	}
	host.Close()
	if _, err := os.Stat(filepath.Join(dir, "desktop-host.json")); !os.IsNotExist(err) {
		t.Fatal("normal exit did not remove activation metadata")
	}
	next, err := AcquireHost(t.Context(), dir, func() bool { return true })
	if err != nil || next == nil {
		t.Fatalf("restart after exit: %v", err)
	}
	next.Close()
}

func TestHostActivationDelegatesForegroundBeforeShowing(t *testing.T) {
	for _, processID := range []uint32{12345, 0, ^uint32(0)} {
		t.Run(fmt.Sprintf("pid=%d", processID), func(t *testing.T) {
			var granted atomic.Uint32
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/activate" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) {
					t.Error("activation lost its authenticated request")
				}
				if processID == 12345 && granted.Load() != processID {
					t.Error("window was shown before delegating foreground eligibility")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			port := server.Listener.Addr().(*net.TCPAddr).Port
			data, err := json.Marshal(hostEndpoint{Port: port, Token: strings.Repeat("a", 64), ProcessID: processID})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "desktop-host.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			allow := func(pid uint32) {
				if pid == 0 || pid == ^uint32(0) {
					t.Fatal("foreground eligibility must target a specific host")
				}
				granted.Store(pid)
			}
			if err := activateHost(t.Context(), server.Client(), path, allow); err != nil || requests.Load() != 1 {
				t.Fatal("activation failed", err)
			}
			if processID != 12345 && granted.Load() != 0 {
				t.Fatal("legacy/invalid PID delegated foreground eligibility")
			}
		})
	}
}

func TestHostActivationRejectsInvalidMetadataBeforeForegroundGrant(t *testing.T) {
	for _, data := range []string{
		`{"port":0,"token":"` + strings.Repeat("a", 64) + `","process_id":12345}`,
		`{"port":12345,"token":"short","process_id":12345}`,
		`{"port":12345,"token":"` + strings.Repeat("a", 64) + `","process_id":-1}`,
		`{"port":12345,"token":"` + strings.Repeat("a", 64) + `","process_id":4294967296}`,
	} {
		path := filepath.Join(t.TempDir(), "desktop-host.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if err := activateHost(t.Context(), http.DefaultClient, path, func(uint32) { t.Fatal("invalid metadata granted foreground eligibility") }); err == nil {
			t.Fatal("invalid metadata accepted")
		}
	}
}

func TestHostInstanceUnavailableDoesNotDuplicate(t *testing.T) {
	dir := t.TempDir()
	lock, err := platform.AcquireLock(filepath.Join(dir, "desktop-host.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if host, err := AcquireHost(ctx, dir, func() bool { t.Fatal("second GUI started"); return true }); err == nil || host != nil {
		t.Fatal("held lock without ready activation endpoint must fail closed")
	}
}

func TestHostInstanceActivationBoundary(t *testing.T) {
	dir := t.TempDir()
	var shows atomic.Int32
	host, err := AcquireHost(t.Context(), dir, func() bool { shows.Add(1); return true })
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	encoded, err := os.ReadFile(filepath.Join(dir, "desktop-host.json"))
	if err != nil {
		t.Fatal(err)
	}
	var endpoint hostEndpoint
	if err := json.Unmarshal(encoded, &endpoint); err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	for _, test := range []struct{ method, path, auth, origin string }{
		{http.MethodPost, "/activate", "", ""},
		{http.MethodPost, "/activate", "Bearer wrong", ""},
		{http.MethodGet, "/activate", "Bearer " + endpoint.Token, ""},
		{http.MethodPost, "/other", "Bearer " + endpoint.Token, ""},
		{http.MethodPost, "/activate", "Bearer " + endpoint.Token, "https://example.test"},
	} {
		request, err := http.NewRequestWithContext(t.Context(), test.method, fmt.Sprintf("http://127.0.0.1:%d%s", endpoint.Port, test.path), nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", test.auth)
		request.Header.Set("Origin", test.origin)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("invalid activation accepted: status=%d", response.StatusCode)
		}
	}
	if shows.Load() != 0 {
		t.Fatal("invalid request activated GUI")
	}
}

func TestHostInstanceAcrossProcessesAndCrash(t *testing.T) {
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, executable, "-test.run=^TestHostInstanceProcessHelper$")
	process.Env = append(os.Environ(), "ANCHOR_TEST_HOST_DIR="+dir, "ANCHOR_TEST_HOST_MODE=hold")
	stdout, err := process.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { process.Process.Kill(); process.Wait() })
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "ready" {
		t.Fatal("first process did not become ready")
	}
	before, err := os.ReadFile(filepath.Join(dir, "desktop-host.json"))
	if err != nil {
		t.Fatal(err)
	}
	second := exec.CommandContext(ctx, executable, "-test.run=^TestHostInstanceProcessHelper$")
	second.Env = append(os.Environ(), "ANCHOR_TEST_HOST_DIR="+dir, "ANCHOR_TEST_HOST_MODE=activate")
	if output, err := second.CombinedOutput(); err != nil {
		t.Fatalf("second process: %v %s", err, output)
	}
	after, err := os.ReadFile(filepath.Join(dir, "desktop-host.json"))
	if err != nil || string(before) != string(after) {
		t.Fatal("second process replaced first GUI's activation endpoint")
	}
	if err := process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	process.Wait()
	// A crash intentionally leaves metadata, but the kernel releases the lock.
	next, err := AcquireHost(ctx, dir, func() bool { return true })
	if err != nil || next == nil {
		t.Fatalf("crashed host prevented restart: %v", err)
	}
	next.Close()
}

func TestHostInstanceProcessHelper(t *testing.T) {
	dir := os.Getenv("ANCHOR_TEST_HOST_DIR")
	if dir == "" {
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	host, err := AcquireHost(ctx, dir, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("ANCHOR_TEST_HOST_MODE") == "activate" {
		if host != nil {
			host.Close()
			t.Fatal("second process became GUI owner")
		}
		return
	}
	if host == nil {
		t.Fatal("first process did not acquire GUI lock")
	}
	defer host.Close()
	fmt.Fprintln(os.Stdout, "ready")
	<-ctx.Done()
}
