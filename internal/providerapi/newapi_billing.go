package providerapi

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"strings"
)

// NewAPI's legacy billing scope is controlled by a private site setting. A
// successful API-key query is useful, but cannot prove account-wallet scope.
func (a *Adapter) getNewAPIBilling(ctx context.Context) (*Allowance, error) {
	if err := a.key(); err != nil {
		return nil, err
	}
	sub, err := a.get(ctx, "/v1/dashboard/billing/subscription", false, nil)
	if err != nil {
		return nil, err
	}
	limit := number(sub, "hard_limit_usd")
	if sub["error"] != nil {
		return nil, failure("upstream_rejected")
	}
	if limit == nil {
		return nil, failure("invalid_response")
	}
	limitValue, err := billingDecimal(*limit)
	if err != nil || limitValue.Sign() < 0 {
		return nil, failure("invalid_response")
	}

	// Upstream uses this placeholder for an unlimited key, not a wallet balance.
	// Suppress it unless an explicit key query proves this key is finite.
	if limitValue.Cmp(big.NewRat(100000000, 1)) == 0 {
		quota, e := a.GetKeyQuota(ctx, 0)
		if e != nil {
			return nil, e
		}
		if quota.Unlimited == nil {
			return nil, failure("unknown")
		}
		if *quota.Unlimited {
			return quota, nil
		}
	}

	usage, err := a.get(ctx, "/v1/dashboard/billing/usage", false, nil)
	if err != nil {
		return nil, err
	}
	used := number(usage, "total_usage")
	if usage["error"] != nil {
		return nil, failure("upstream_rejected")
	}
	if used == nil {
		return nil, failure("invalid_response")
	}
	usedValue, err := billingDecimal(*used)
	if err != nil || usedValue.Sign() < 0 {
		return nil, failure("invalid_response")
	}
	// The compatibility usage endpoint reports hundredths of the same unit.
	usedValue.Quo(usedValue, big.NewRat(100, 1))
	remaining := new(big.Rat).Sub(limitValue, usedValue)

	unit := "site_unit"
	if a.publicStatus == nil {
		status := a.status
		raw, e := a.getWithAuth(ctx, "/api/status", false, true, nil)
		if e == nil {
			a.publicStatus = data(raw)
		} else if !billingUnavailable(e) && !publicStatusDenied(e) {
			return nil, e
		} else {
			a.status = status
		}
	}
	switch text(a.publicStatus, "quota_display_type") {
	case "USD":
		unit = "USD"
	case "CNY":
		unit = "CNY"
	case "TOKENS":
		unit = "quota"
	}
	out := allowance("billing")
	out.Remaining = &Amount{Value: billingNumber(remaining), Unit: unit}
	out.Used = &Amount{Value: billingNumber(usedValue), Unit: unit}
	out.Limit = &Amount{Value: *limit, Unit: unit}
	return out, nil
}

// Only missing/incompatible endpoints permit a summary to fall back to key
// quota. Authentication, permission, limits, network and server errors survive.
func billingUnavailable(err error) bool {
	var f *Failure
	return errors.As(err, &f) && (f.Code == "unsupported" || f.Code == "invalid_response")
}

func publicStatusDenied(err error) bool {
	var f *Failure
	return errors.As(err, &f) && (f.Code == "authentication" || f.Code == "permission_denied")
}

func billingDecimal(n json.Number) (*big.Rat, error) {
	s := n.String()
	if len(s) > 128 {
		return nil, failure("invalid_response")
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		e, err := strconv.Atoi(s[i+1:])
		if err != nil || e < -128 || e > 128 {
			return nil, failure("invalid_response")
		}
	}
	v, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, failure("invalid_response")
	}
	return v, nil
}

func billingNumber(v *big.Rat) json.Number {
	// Inputs are bounded decimals with at most 128 mantissa characters and a
	// 128 exponent. Division by 100 stays exact within 258 decimal places.
	s := strings.TrimRight(strings.TrimRight(v.FloatString(258), "0"), ".")
	if s == "-0" {
		s = "0"
	}
	return json.Number(s)
}
