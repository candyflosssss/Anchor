package desktop

import (
	"context"
	"time"
)

func (b *Bridge) CheckRemoteUpdate(instanceID string) UpdateResult {
	s := b.session
	if !s.mu.TryLock() {
		return UpdateResult{Contract: Contract, Problem: &Problem{"conflict"}}
	}
	defer s.mu.Unlock()
	out := UpdateResult{Contract: Contract}
	s.remoteUpdateCheck = nil
	s.remoteUpdateInstance = ""
	if s.remote == nil || instanceID == "" || s.remote.Profile.InstanceID != instanceID {
		out.Problem = &Problem{"identity"}
		return out
	}
	if s.updating {
		out.Problem = &Problem{"conflict"}
		return out
	}
	ctx, cancel := context.WithTimeout(s.ctx, 45*time.Second)
	defer cancel()
	checked, err := s.remote.CheckCoreUpdate(ctx)
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	s.remoteUpdateCheck = &checked
	s.remoteUpdateInstance = instanceID
	out.Data = updateInfo(checked)
	return out
}

func (b *Bridge) InstallRemoteUpdate(in ConfirmRemoteUpdate) UpdateResult {
	s := b.session
	if !s.mu.TryLock() {
		return UpdateResult{Contract: Contract, Problem: &Problem{"conflict"}}
	}
	defer s.mu.Unlock()
	out := UpdateResult{Contract: Contract}
	if !in.Approved || s.updating || s.remoteUpdateCheck == nil || !s.remoteUpdateCheck.Available || in.Version != s.remoteUpdateCheck.Version {
		out.Problem = &Problem{"invalid"}
		return out
	}
	if s.remote == nil || in.InstanceID == "" || s.remote.Profile.InstanceID != in.InstanceID || s.remoteUpdateInstance != in.InstanceID {
		out.Problem = &Problem{"identity"}
		return out
	}
	checked := *s.remoteUpdateCheck
	s.remoteUpdateCheck = nil
	ctx, cancel := context.WithTimeout(s.ctx, 6*time.Minute)
	defer cancel()
	result, err := s.remote.UpgradeCore(ctx, checked)
	if err != nil {
		out.Problem = problem(err, true)
		return out
	}
	out.Data = updateInfo(checked)
	out.Data.CurrentVersion = result.Version
	out.Data.Available = false
	out.Data.Phase = "applied"
	return out
}
