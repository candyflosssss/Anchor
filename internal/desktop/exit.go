package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/platform"
)

// ExitWithLocalCore is a host-only tray action, not a JavaScript binding. It
// always targets this host's local data directory, even when a remote is active.
// confirm receives the authenticated instance ID, or an empty ID if no local
// core holds the lifetime lock. Cancellation and failures leave the host open.
func (s *Session) ExitWithLocalCore(confirm func(instanceID string) bool) StopResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := StopResult{Contract: Contract}
	if s.updating {
		out.Problem = &Problem{"conflict"}
		return out
	}
	if s.options.Quit == nil || confirm == nil {
		out.Problem = &Problem{"unsupported"}
		return out
	}
	if s.ctx.Err() != nil {
		out.Problem = &Problem{"closed"}
		return out
	}
	finish := func() StopResult {
		// Prevent a queued Start from replacing the core before Quit runs.
		s.cancel()
		if s.remote == nil {
			s.cfg = nil
		}
		s.options.Quit()
		return StopResult{Contract: Contract, Data: &StopInfo{Accepted: true}}
	}
	// Share the update/startup guard; never start a core just to stop it.
	startLock, err := platform.AcquireLock(filepath.Join(s.options.Dir, "desktop-start.lock"))
	if errors.Is(err, os.ErrNotExist) {
		if !confirm("") {
			out.Data = &StopInfo{Accepted: false}
			return out
		}
		// A concurrently initialized directory was not the target the user
		// confirmed. Leave the host open instead of claiming it is stopped.
		if _, err := os.Stat(s.options.Dir); !errors.Is(err, os.ErrNotExist) {
			out.Problem = &Problem{"conflict"}
			return out
		}
		return finish()
	}
	if err != nil {
		out.Problem = &Problem{"conflict"}
		return out
	}
	defer startLock.Close()
	coreLock, _ := platform.AcquireLock(filepath.Join(s.options.Dir, "instance.lock"))
	defer func() {
		if coreLock != nil {
			coreLock.Close()
		}
	}()
	var instanceID string
	if coreLock == nil {
		ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
		cl, id, p := s.localStopConnection(ctx, "")
		cancel()
		if p != nil {
			out.Problem = p
			return out
		}
		cl.Close()
		instanceID = id
	}
	if !confirm(instanceID) {
		out.Data = &StopInfo{Accepted: false}
		return out
	}
	if coreLock == nil {
		// The user may keep the dialog open indefinitely. Recheck both local
		// state and authenticated identity after approval, with a fresh deadline.
		coreLock, _ = platform.AcquireLock(filepath.Join(s.options.Dir, "instance.lock"))
		if coreLock == nil {
			ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
			defer cancel()
			cl, _, p := s.localStopConnection(ctx, instanceID)
			if p != nil {
				out.Problem = p
				return out
			}
			defer cl.Close()
			if err := cl.Stop(ctx); err != nil {
				out.Problem = problem(err, true)
				return out
			}
			if s.remote == nil {
				s.cfg = nil
			}
			// Acceptance alone is not shutdown. Wait for the core's normal
			// draining/recovery path to release its lifetime lock; never kill it.
			for coreLock == nil {
				if !pause(ctx) {
					out.Problem = &Problem{"conflict"}
					return out
				}
				coreLock, _ = platform.AcquireLock(filepath.Join(s.options.Dir, "instance.lock"))
			}
		}
	}
	return finish()
}

func (s *Session) localStopConnection(ctx context.Context, expected string) (*client.Client, string, *Problem) {
	cfg, token, err := s.readLocal()
	if err != nil {
		return nil, "", problem(err, false)
	}
	if (expected != "" && cfg.InstanceID != expected) || (s.remote == nil && s.cfg != nil && s.cfg.InstanceID != cfg.InstanceID) {
		return nil, "", &Problem{"identity"}
	}
	cl, info, err := connect(ctx, cfg, token)
	if err != nil {
		return nil, "", problem(err, false)
	}
	if !slices.Contains(info.Capabilities, "graceful_stop") {
		cl.Close()
		return nil, "", &Problem{"unsupported"}
	}
	return cl, info.InstanceID, nil
}
