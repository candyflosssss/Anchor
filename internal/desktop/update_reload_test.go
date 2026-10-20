package desktop

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/hostipc"
)

func TestRuntimeReloadIsAuthenticatedAndDataDirScoped(t *testing.T) {
	hostDir, dataDir := t.TempDir(), t.TempDir()
	var reloads atomic.Int32
	host, err := AcquireHostWithReload(t.Context(), hostDir, func() bool { return true }, dataDir, func() bool { reloads.Add(1); return true })
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	if running, err := hostipc.Reload(context.Background(), hostDir, t.TempDir()); err != nil || running || reloads.Load() != 0 {
		t.Fatal("cross-directory reload", running, err)
	}
	if running, err := hostipc.Reload(context.Background(), hostDir, dataDir); err != nil || !running || reloads.Load() != 1 {
		t.Fatal("reload failed", running, err)
	}
	host.Close()
	if running, err := hostipc.Reload(context.Background(), hostDir, dataDir); err != nil || running || reloads.Load() != 1 {
		t.Fatal("started a closed GUI", running, err)
	}
}
