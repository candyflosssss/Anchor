package desktop

import (
	"context"
	"time"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/updateflow"
	"github.com/candyflosssss/Anchor/internal/updater"
)

var runtimeHelperLauncher = launchUpdateHelper

// Caller holds session and update locks. Stage and validate both the runtime
// and window helper before the only service restart.
func (s *Session) installRuntime(service *updater.Service, d updater.Download) UpdateResult {
	out := UpdateResult{Contract: Contract}
	ctx, cancel := context.WithTimeout(s.ctx, 50*time.Second)
	defer cancel()
	approval := updater.InstallApproval{Version: d.Check.Version, SHA256: d.Check.Artifact.SHA256}
	r, err := service.PrepareRuntime(ctx, s.options.Dir, approval)
	if err != nil {
		out.Problem = updateProblem(err)
		return out
	}
	if err := updater.LaunchInstallerHelper(s.options.Dir, s.options.Executable, approval, runtimeHelperLauncher); err != nil {
		out.Problem = updateProblem(err)
		return out
	}
	expected := ""
	if s.remote == nil && s.cfg != nil {
		expected = s.cfg.InstanceID
	}
	if _, err := updateflow.Switch(ctx, s.options.Dir, service, r, expected, 0); err != nil {
		out.Problem = problem(err, true)
		return out
	}
	s.updating = true
	out.Data = updateInfo(d.Check)
	out.Data.Phase = "installing"
	out.Data.DownloadPath = d.Path
	s.options.Quit()
	return out
}

// ReloadUpdatedDesktop is a host-only, authenticated IPC action after a CLI
// update. It does not restart the already upgraded core a second time.
func (s *Session) ReloadUpdatedDesktop() bool {
	if !s.updateMu.TryLock() {
		return false
	}
	defer s.updateMu.Unlock()
	if !s.mu.TryLock() {
		return false
	}
	defer s.mu.Unlock()
	if s.updating || s.options.Quit == nil {
		return false
	}
	active, err := updater.ActiveRuntime(s.options.Dir, "desktop", core.Version)
	if err != nil || active.Path == "" {
		return false
	}
	lock, err := updater.Lock(s.options.Dir)
	if err != nil {
		return false
	}
	defer lock.Close()
	if err := updater.LaunchInstallerHelper(s.options.Dir, s.options.Executable, updater.InstallApproval{Version: active.Version, SHA256: active.Artifact.SHA256}, launchUpdateHelper); err != nil {
		return false
	}
	s.updating = true
	s.options.Quit()
	return true
}
