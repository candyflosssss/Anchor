// Package hostipc is the restricted current-user desktop activation transport.
package hostipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/candyflosssss/Anchor/internal/platform"
)

func Reload(ctx context.Context, hostDir, dataDir string) (bool, error) {
	lock, err := platform.AcquireLock(filepath.Join(hostDir, "desktop-host.lock"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err == nil {
		lock.Close()
		return false, nil
	}
	path := filepath.Join(hostDir, "desktop-host.json")
	if err := platform.CheckManagedPath(hostDir, path); err != nil {
		return true, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return true, err
	}
	if st.Size() > 4096 {
		return true, errors.New("invalid desktop metadata")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return true, err
	}
	var endpoint struct {
		Port  int    `json:"port"`
		Token string `json:"token"`
	}
	if json.Unmarshal(raw, &endpoint) != nil || endpoint.Port < 1 || endpoint.Port > 65535 || len(endpoint.Token) != 64 {
		return true, errors.New("invalid desktop metadata")
	}
	dir, err := filepath.Abs(dataDir)
	if err != nil {
		return true, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/reload-update", endpoint.Port), nil)
	if err != nil {
		return true, err
	}
	req.Header.Set("Authorization", "Bearer "+endpoint.Token)
	req.Header.Set("X-Anchor-Data-Dir", dir)
	tr := &http.Transport{Proxy: nil}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusPreconditionFailed {
		return false, nil
	} // another data directory's GUI
	if resp.StatusCode != http.StatusNoContent {
		return true, errors.New("desktop could not reload; close and reopen its window")
	}
	return true, nil
}
