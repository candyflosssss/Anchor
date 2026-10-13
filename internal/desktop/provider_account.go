package desktop

import (
	"context"
	"time"

	"github.com/candyflosssss/Anchor/internal/providerapi"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (b *Bridge) ProviderAccountSettings(in WorkspaceQuery) AccountSettingsResult {
	if !state.ValidProviderID(in.ProviderID) {
		return AccountSettingsResult{Contract: Contract, Problem: &Problem{"invalid"}}
	}
	return b.accountSettingsCall(in.ProviderID, nil)
}
func (b *Bridge) SaveProviderAccount(in SaveAccountSettings) AccountSettingsResult {
	cfg := providerapi.Config{Platform: in.Platform, SiteURL: in.SiteURL, UserID: in.UserID}
	if in.UserToken != nil {
		cfg.UserToken = *in.UserToken
	}
	if !state.ValidProviderID(in.ProviderID) || in.ExpectedRevision < 1 || in.ExpectedRevision >= MaxSafeRevision || in.UserID > MaxSafeRevision || in.Platform == "" || cfg.Validate() != nil {
		return AccountSettingsResult{Contract: Contract, Problem: &Problem{"invalid"}}
	}
	return b.accountSettingsCall(in.ProviderID, &providerapi.Configure{ProviderID: in.ProviderID, ExpectedRevision: in.ExpectedRevision, Config: cfg, UserToken: in.UserToken})
}
func (b *Bridge) accountSettingsCall(id string, write *providerapi.Configure) AccountSettingsResult {
	out := AccountSettingsResult{Contract: Contract}
	s := b.session
	s.mu.Lock()
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	cl, _, err := s.connection(ctx)
	s.mu.Unlock()
	defer cancel()
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	var v providerapi.Settings
	if write == nil {
		v, err = cl.ProviderPlatform(ctx, id)
	} else {
		v, err = cl.ConfigureProviderPlatform(ctx, *write)
	}
	if err != nil {
		out.Problem = problem(err, write != nil)
		return out
	}
	if v.ProviderID != id || v.Revision < 1 || v.Revision > MaxSafeRevision || v.Config.UserID < 0 || v.Config.UserID > MaxSafeRevision {
		code := "read_error"
		if write != nil {
			code = "unknown"
		}
		out.Problem = &Problem{code}
		return out
	}
	out.Data = &AccountSettings{ProviderID: v.ProviderID, Revision: v.Revision, Platform: v.Config.Platform, SiteURL: v.Config.SiteURL, UserID: v.Config.UserID, HasUserToken: v.Config.HasUserToken}
	return out
}
func (b *Bridge) ProviderAllowance(in AccountQuery) AccountObservationResult {
	out := AccountObservationResult{Contract: Contract}
	if !state.ValidProviderID(in.ProviderID) || in.ExpectedRevision < 1 || in.ExpectedRevision > MaxSafeRevision || in.RemoteKeyID < 0 || in.RemoteKeyID > MaxSafeRevision || (in.KeyID != "" && !state.ValidID(in.KeyID)) || (in.Operation != "balance" && in.Operation != "key-quota") {
		out.Problem = &Problem{"invalid"}
		return out
	}
	s := b.session
	s.mu.Lock()
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	cl, _, err := s.connection(ctx)
	s.mu.Unlock()
	defer cancel()
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	defer cl.Close()
	v, err := cl.QueryProvider(ctx, providerapi.Query{ProviderID: in.ProviderID, ExpectedRevision: in.ExpectedRevision, Operation: in.Operation, KeyID: in.KeyID, RemoteKeyID: in.RemoteKeyID})
	if err != nil {
		out.Problem = problem(err, false)
		return out
	}
	if v.ProviderID != in.ProviderID || v.Revision != in.ExpectedRevision || v.Operation != in.Operation || v.RemoteKeyID != in.RemoteKeyID {
		out.Problem = &Problem{"read_error"}
		return out
	}
	a := v.Balance
	if in.Operation == "key-quota" {
		a = v.KeyQuota
	}
	out.Data = &AccountObservation{ProviderID: v.ProviderID, Revision: v.Revision, Platform: v.Platform, Operation: v.Operation, KeyID: v.KeyID, RemoteKeyID: v.RemoteKeyID, ObservedAt: v.ObservedAt, HTTPStatus: v.HTTPStatus, Failure: v.Failure, Allowance: accountAllowance(a)}
	return out
}
func accountAmount(v *providerapi.Amount) *AccountAmount {
	if v == nil {
		return nil
	}
	return &AccountAmount{Value: v.Value.String(), Unit: v.Unit}
}
func accountAllowance(v *providerapi.Allowance) *AccountAllowance {
	if v == nil {
		return nil
	}
	out := &AccountAllowance{Scope: v.Scope, Remaining: accountAmount(v.Remaining), Used: accountAmount(v.Used), Limit: accountAmount(v.Limit), Unlimited: v.Unlimited, Windows: []AccountWindow{}}
	for _, w := range v.Windows {
		out.Windows = append(out.Windows, AccountWindow{Period: w.Period, Remaining: accountAmount(w.Remaining), Used: accountAmount(w.Used), Limit: accountAmount(w.Limit), ResetAt: w.ResetAt})
	}
	return out
}
