package desktop

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/candyflosssss/Anchor/internal/platform"
)

// HostInstance owns the GUI only. Its directory is the current user's default
// Anchor directory, independent of the selected core's --data-dir. Core and
// installer-helper processes never acquire this lock.
type HostInstance struct {
	lock     *platform.Lock
	server   *http.Server
	endpoint string
	once     sync.Once
}

type hostEndpoint struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// AcquireHost starts the first GUI's activation listener, or activates the
// existing GUI and returns nil. Never fall back to a second GUI when activation
// fails. Retrying the kernel lock also permits recovery after a crashed host.
// activate queues requests until the native window is ready and rejects them
// once the host starts exiting. No launch arguments are sent to the incumbent.
func AcquireHost(ctx context.Context, dir string, activate func() bool) (*HostInstance, error) {
	if err := platform.SecureDir(dir); err != nil {
		return nil, err
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport, Timeout: 500 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	endpoint := filepath.Join(dir, "desktop-host.json")
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("existing desktop did not respond: %w", err)
		}
		lock, err := platform.AcquireLock(filepath.Join(dir, "desktop-host.lock"))
		if err == nil {
			return startHost(lock, endpoint, activate)
		}
		if activateHost(ctx, client, endpoint) == nil {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("existing desktop did not respond: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func startHost(lock *platform.Lock, endpoint string, activate func() bool) (_ *HostInstance, err error) {
	defer func() {
		if err != nil {
			lock.Close()
		}
	}()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			listener.Close()
		}
	}()
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		return nil, err
	}
	data := hostEndpoint{Port: listener.Addr().(*net.TCPAddr).Port, Token: hex.EncodeToString(secret[:])}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	if err = platform.SecureFile(endpoint); err != nil {
		return nil, err
	}
	// Readers retry incomplete metadata; the kernel lock stays held throughout.
	if err = os.WriteFile(endpoint, encoded, 0600); err != nil {
		return nil, err
	}
	server := &http.Server{
		ReadHeaderTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second,
		MaxHeaderBytes: 4096,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/activate" || r.Method != http.MethodPost || r.Header.Get("Origin") != "" ||
				subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+data.Token)) != 1 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if !activate() {
				w.WriteHeader(http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	instance := &HostInstance{lock: lock, server: server, endpoint: endpoint}
	go server.Serve(listener)
	return instance, nil
}

func activateHost(ctx context.Context, client *http.Client, path string) error {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var data hostEndpoint
	if json.Unmarshal(encoded, &data) != nil || data.Port < 1 || data.Port > 65535 || len(data.Token) != 64 {
		return errors.New("desktop activation metadata is not ready")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/activate", data.Port), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+data.Token)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return errors.New("desktop activation was not accepted")
	}
	return nil
}

func (h *HostInstance) Close() {
	h.once.Do(func() {
		h.server.Close()
		os.Remove(h.endpoint)
		// Release last so a replacement host cannot lose its own metadata.
		h.lock.Close()
	})
}
