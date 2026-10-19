package providerapi

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
)

// The read-only token endpoint can work even when normal token authentication
// rejects an exhausted/expired key. Recent sites optionally expose a wallet in
// user_quota; its absence must never turn a key cap into an account balance.
func (a *Adapter) getNewAPIBalance(ctx context.Context) (*Allowance, error) {
	m, tokenErr := a.getNewAPITokenUsage(ctx)
	if tokenErr == nil && m["user_quota"] != nil && string(m["user_quota"]) != "null" {
		var wallet object
		if json.Unmarshal(m["user_quota"], &wallet) != nil || wallet == nil {
			return nil, failure("invalid_response")
		}
		out := allowance("account")
		out.Remaining = amount(wallet, "total_available", "quota")
		out.Used = amount(wallet, "total_used", "quota")
		if out.Remaining == nil {
			return nil, failure("invalid_response")
		}
		return out, a.normalizeNewAPIQuota(ctx, out)
	}
	if tokenErr != nil && !newAPIAlternative(tokenErr) {
		return nil, tokenErr
	}
	out, err := a.getNewAPIBilling(ctx)
	if err == nil || !newAPIAlternative(err) {
		return out, err
	}
	if tokenErr != nil {
		if billingUnavailable(err) && !billingUnavailable(tokenErr) {
			return nil, tokenErr
		}
		return nil, err
	}
	out, err = newAPIKeyQuota(m)
	if err == nil {
		a.status = http.StatusOK
		err = a.normalizeNewAPIQuota(ctx, out)
	}
	return out, err
}

func newAPIAlternative(err error) bool {
	var f *Failure
	return errors.As(err, &f) && (f.Code == "unsupported" || f.Code == "invalid_response" ||
		f.Code == "authentication" || f.Code == "permission_denied" || f.Code == "upstream_rejected")
}

func (a *Adapter) getNewAPITokenUsage(ctx context.Context) (object, error) {
	if err := a.key(); err != nil {
		return nil, err
	}
	if a.tokenUsage != nil {
		return a.tokenUsage, nil
	}
	raw, err := a.get(ctx, "/api/usage/token/", false, nil)
	if err != nil {
		return nil, err
	}
	a.tokenUsage = data(raw)
	if a.tokenUsage == nil {
		return nil, failure("invalid_response")
	}
	return a.tokenUsage, nil
}

func newAPIKeyQuota(m object) (*Allowance, error) {
	out := allowance("key")
	out.Remaining = amount(m, "total_available", "quota")
	out.Used = amount(m, "total_used", "quota")
	out.Limit = amount(m, "total_granted", "quota")
	out.Unlimited = boolean(m, "unlimited_quota")
	if out.Remaining == nil {
		return nil, failure("invalid_response")
	}
	return out, nil
}

func (a *Adapter) newAPIStatus(ctx context.Context) error {
	if a.publicStatus != nil {
		return nil
	}
	status := a.status
	raw, err := a.getWithAuth(ctx, "/api/status", false, true, nil)
	if err == nil {
		a.publicStatus = data(raw)
	} else if !billingUnavailable(err) && !publicStatusDenied(err) {
		return err
	}
	// Metadata must not replace the status of the successful balance query.
	a.status = status
	return nil
}

// Raw quotas and legacy billing have different units. Only raw quotas are
// converted; billing has already applied the site's divisor and exchange rate.
// Missing/unknown metadata leaves raw quota intact, never a guessed currency.
func (a *Adapter) normalizeNewAPIQuota(ctx context.Context, out *Allowance) error {
	if err := a.newAPIStatus(ctx); err != nil {
		return err
	}
	unit := text(a.publicStatus, "quota_display_type")
	if unit != "USD" && unit != "CNY" {
		return nil
	}
	divisor := number(a.publicStatus, "quota_per_unit")
	if divisor == nil {
		return nil
	}
	d, err := billingDecimal(*divisor)
	if err != nil || d.Sign() <= 0 {
		return nil
	}
	factor := new(big.Rat).Inv(d)
	if unit == "CNY" {
		rate := number(a.publicStatus, "usd_exchange_rate")
		if rate == nil {
			return nil
		}
		r, err := billingDecimal(*rate)
		if err != nil || r.Sign() <= 0 {
			return nil
		}
		factor.Mul(factor, r)
	}
	for _, value := range []*Amount{out.Remaining, out.Used, out.Limit} {
		if value == nil {
			continue
		}
		n, err := billingDecimal(value.Value)
		if err != nil {
			return err
		}
		value.Value = billingNumber(n.Mul(n, factor))
		value.Unit = unit
	}
	return nil
}
