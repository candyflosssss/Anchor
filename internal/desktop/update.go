package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/updater"
)

func updateProblem(err error) *Problem {
	switch {
	case errors.Is(err, updater.ErrIntegrity):
		return &Problem{"integrity"}
	case errors.Is(err, updater.ErrInvalid):
		return &Problem{"invalid"}
	case errors.Is(err, updater.ErrUnavailable):
		return &Problem{"unsupported"}
	case errors.Is(err, updater.ErrBusy):
		return &Problem{"conflict"}
	case errors.Is(err, os.ErrPermission):
		return &Problem{"permission"}
	default:
		return problem(err, false)
	}
}
func updateInfo(c updater.Check) *UpdateInfo {
	info := &UpdateInfo{CurrentVersion: c.CurrentVersion, LatestVersion: c.Version, Channel: c.Channel, Source: c.Source, Available: c.Available, Phase: "checked"}
	if c.Artifact != nil {
		info.InstallKind = c.Artifact.Kind
	}
	return info
}
func (b *Bridge) UpdateStatus() UpdateResult {
	s := b.session
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	r := UpdateResult{Contract: Contract}
	cfg, err := updater.ReadConfigForVersion(s.options.Dir, core.Version)
	if err != nil {
		r.Problem = updateProblem(err)
		return r
	}
	source := cfg.ManifestURL
	if source == "" {
		source = "https://github.com/" + updater.Repository + "/releases"
	}
	r.Data = &UpdateInfo{CurrentVersion: core.Version, Channel: cfg.Channel, Source: source, Phase: "idle"}
	d, err := updater.New("desktop", core.Version).ReadDownload(s.options.Dir)
	if err == nil {
		if d.Check.Channel == cfg.Channel && d.Check.Source == source {
			r.Data = updateInfo(d.Check)
			r.Data.CurrentVersion = core.Version
			r.Data.Phase = "downloaded"
			r.Data.DownloadPath = d.Path
		}
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, updater.ErrUnavailable) {
		r.Problem = updateProblem(err)
		r.Data = nil
	}
	return r
}
func (b *Bridge) CheckUpdate(channel string) UpdateResult {
	s := b.session
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	r := UpdateResult{Contract: Contract}
	s.updateCheck = nil
	cfg, err := updater.ReadConfigForVersion(s.options.Dir, core.Version)
	if err != nil {
		r.Problem = updateProblem(err)
		return r
	}
	cfg.Channel = channel
	if err := updater.Configure(s.options.Dir, cfg); err != nil {
		r.Problem = updateProblem(err)
		return r
	}
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	c, err := updater.New("desktop", core.Version).Check(ctx, cfg)
	if err != nil {
		r.Problem = updateProblem(err)
		return r
	}
	s.updateCheck = &c
	r.Data = updateInfo(c)
	return r
}
func (b *Bridge) DownloadUpdate(version string) UpdateResult {
	s := b.session
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	r := UpdateResult{Contract: Contract}
	if s.updateCheck == nil || version != s.updateCheck.Version || !s.updateCheck.Available {
		r.Problem = &Problem{"invalid"}
		return r
	}
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
	defer cancel()
	d, err := updater.New("desktop", core.Version).Download(ctx, s.options.Dir, *s.updateCheck)
	if err != nil {
		r.Problem = updateProblem(err)
		return r
	}
	r.Data = updateInfo(d.Check)
	r.Data.Phase = "downloaded"
	r.Data.DownloadPath = d.Path
	return r
}
func (b *Bridge) InstallUpdate(in ConfirmUpdate) UpdateResult {
	s := b.session
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	r := UpdateResult{Contract: Contract}
	if !in.Approved || s.options.Quit == nil || s.updating {
		r.Problem = &Problem{"invalid"}
		return r
	}
	updateLock, err := updater.Lock(s.options.Dir)
	if err != nil {
		r.Problem = updateProblem(err)
		return r
	}
	defer updateLock.Close()
	service := updater.New("desktop", core.Version)
	d, err := service.ReadDownload(s.options.Dir)
	if err != nil {
		r.Problem = updateProblem(err)
		return r
	}
	if d.Check.Version != in.Version {
		r.Problem = &Problem{"invalid"}
		return r
	}
	if d.Check.Artifact.Kind == "runtime" {
		return s.installRuntime(service, d)
	}
	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
	defer cancel()
	// Prevent this host and other desktop windows from starting a replacement
	// core while shutdown is in flight. No force-stop on timeout or mismatch.
	startLock, err := platform.AcquireLock(filepath.Join(s.options.Dir, "desktop-start.lock"))
	if err != nil {
		r.Problem = &Problem{"conflict"}
		return r
	}
	defer startLock.Close()
	coreLock, lockErr := platform.AcquireLock(filepath.Join(s.options.Dir, "instance.lock"))
	if lockErr != nil {
		cfg, token, err := s.readLocal()
		if err != nil {
			r.Problem = problem(err, false)
			return r
		}
		if s.remote == nil && s.cfg != nil && s.cfg.InstanceID != cfg.InstanceID {
			r.Problem = &Problem{"identity"}
			return r
		}
		cl, info, err := connect(ctx, cfg, token)
		if err != nil {
			r.Problem = problem(err, false)
			return r
		}
		defer cl.Close()
		if !slices.Contains(info.Capabilities, "restart_handoff") {
			r.Problem = &Problem{"unsupported"}
			return r
		}
		if err := cl.StopForRestart(ctx); err != nil {
			r.Problem = problem(err, true)
			return r
		}
		if s.remote == nil {
			s.cfg = nil
		}
		for coreLock == nil {
			if !pause(ctx) {
				r.Problem = &Problem{"conflict"}
				return r
			}
			coreLock, _ = platform.AcquireLock(filepath.Join(s.options.Dir, "instance.lock"))
		}
	}
	defer coreLock.Close()
	if err := updater.LaunchInstallerHelper(s.options.Dir, s.options.Executable, updater.InstallApproval{Version: d.Check.Version, SHA256: d.Check.Artifact.SHA256}, launchUpdateHelper); err != nil {
		r.Problem = updateProblem(err)
		return r
	}
	s.updating = true
	s.cfg = nil
	r.Data = updateInfo(d.Check)
	r.Data.Phase = "installing"
	r.Data.DownloadPath = d.Path
	// The host callback schedules Quit, allowing the result to be sent first.
	s.options.Quit()
	return r
}
