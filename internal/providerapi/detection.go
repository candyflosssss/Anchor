package providerapi

import (
	"context"
	"errors"
)

// DetectPlatform uses endpoint schemas, never a hostname, logo or inference
// protocol. Public probes carry no credential. All probes share Query's budget.
func (a *Adapter) DetectPlatform(ctx context.Context) (string, error) {
	probes := []struct {
		path     string
		public   bool
		identify func(object) string
	}{
		{"/api/status", true, func(raw object) string {
			m := data(raw)
			if text(m, "version") == "" || number(m, "quota_per_unit") == nil {
				return ""
			}
			if boolean(m, "default_use_auto_group") != nil && boolean(m, "enable_batch_update") != nil {
				return "newapi"
			}
			if boolean(m, "oidc") != nil && m["oidc_well_known"] != nil {
				return "oneapi"
			}
			return ""
		}},
		{"/api/v1/settings/public", true, func(raw object) string {
			m := data(raw)
			if boolean(m, "registration_enabled") != nil && text(m, "site_name") != "" && text(m, "version") != "" && m["api_base_url"] != nil {
				return "sub2api"
			}
			return ""
		}},
		{sub2KeyBillingPath, false, func(m object) string {
			if sub2KeyBillingSchema(m) {
				a.keyBilling = m
				return "sub2api"
			}
			return ""
		}},
		{"/v1/usage", false, func(m object) string {
			if text(m, "mode") == "unrestricted" && amount(m, "balance", "USD") != nil {
				return "sub2api"
			}
			if text(m, "mode") == "quota_limited" && (m["quota"] != nil || m["rate_limits"] != nil) {
				return "sub2api"
			}
			return ""
		}},
		{"/api/usage/token/", false, func(raw object) string {
			m := data(raw)
			if number(m, "total_available") != nil && number(m, "total_used") != nil && number(m, "total_granted") != nil {
				return "newapi"
			}
			return ""
		}},
	}
	var denied error
	for _, p := range probes {
		if !p.public && a.Key == "" {
			continue
		}
		m, err := a.getWithAuth(ctx, p.path, false, p.public, nil)
		if err != nil {
			var f *Failure
			if !errors.As(err, &f) {
				return "", err
			}
			switch f.Code {
			case "timeout", "network", "too_large", "rate_limited":
				return "", err
			case "authentication", "permission_denied":
				if !p.public {
					denied = err
				}
			}
			continue
		}
		if p.path == "/api/status" {
			a.publicStatus = data(m)
		}
		if platform := p.identify(m); platform != "" {
			return platform, nil
		}
	}
	if denied != nil {
		return "", denied
	}
	return "", failure("platform_unknown")
}

// GetBalanceSummary prefers an explicitly configured NewAPI account token.
// Without it, the selected inference key exposes site-controlled billing or a
// key cap. Account authorization failures must not fall back to key figures.
func (a *Adapter) GetBalanceSummary(ctx context.Context) (*Allowance, error) {
	if a.Config.Platform == "newapi" && a.Config.UserToken != "" {
		return a.GetBalance(ctx)
	}
	if err := a.key(); err != nil {
		return nil, err
	}
	switch a.Config.Platform {
	case "sub2api":
		m, err := a.get(ctx, "/v1/usage", false, nil)
		if err != nil {
			return nil, err
		}
		if text(m, "mode") == "unrestricted" {
			out := allowance("account")
			out.Remaining = amount(m, "balance", "USD")
			if out.Remaining == nil {
				return nil, failure("unknown")
			}
			return out, nil
		}
		return sub2KeyQuota(m)
	case "newapi":
		out, err := a.getNewAPIBilling(ctx)
		if billingUnavailable(err) {
			return a.GetKeyQuota(ctx, 0)
		}
		return out, err
	case "oneapi":
		return nil, failure("auth_required")
	default:
		return nil, failure("platform_unknown")
	}
}
