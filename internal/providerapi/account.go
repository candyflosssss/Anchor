package providerapi

import (
	"context"
	"encoding/json"
)

func (a *Adapter) GetBalance(ctx context.Context) (*Allowance, error) {
	out := allowance("account")
	if a.Config.Platform == "newapi" && a.Config.UserToken == "" {
		return a.getNewAPIBilling(ctx)
	}
	if a.Config.Platform == "sub2api" && a.Config.UserToken == "" {
		if err := a.key(); err != nil {
			return nil, err
		}
		m, err := a.get(ctx, "/v1/usage", false, nil)
		if err != nil {
			return nil, err
		}
		// remaining alone can be a key cap or subscription. Only balance is a wallet.
		if text(m, "mode") != "unrestricted" || amount(m, "balance", "USD") == nil {
			return nil, failure("auth_required")
		}
		out.Remaining = amount(m, "balance", "USD")
		return out, nil
	}
	if err := a.user(); err != nil {
		return nil, err
	}
	path := "/api/user/self"
	unit := "quota"
	field := "quota"
	if a.Config.Platform == "sub2api" {
		path = "/api/v1/user/profile"
		unit = "USD"
		field = "balance"
	}
	raw, err := a.get(ctx, path, true, nil)
	if err != nil {
		return nil, err
	}
	m := data(raw)
	out.Remaining = amount(m, field, unit)
	if out.Remaining == nil {
		return nil, failure("invalid_response")
	}
	if a.Config.Platform != "sub2api" {
		out.Used = amount(m, "used_quota", unit)
	}
	return out, nil
}

func (a *Adapter) GetKeyQuota(ctx context.Context, id int64) (*Allowance, error) {
	out := allowance("key")
	switch a.Config.Platform {
	case "sub2api":
		if err := a.key(); err != nil {
			return nil, err
		}
		m, err := a.get(ctx, "/v1/usage", false, nil)
		if err != nil {
			return nil, err
		}
		return sub2KeyQuota(m)
	case "newapi":
		if err := a.key(); err != nil {
			return nil, err
		}
		raw, err := a.get(ctx, "/api/usage/token/", false, nil)
		if err != nil {
			return nil, err
		}
		m := data(raw)
		out.Remaining = amount(m, "total_available", "quota")
		out.Used = amount(m, "total_used", "quota")
		out.Limit = amount(m, "total_granted", "quota")
		out.Unlimited = boolean(m, "unlimited_quota")
		if out.Remaining == nil {
			return nil, failure("invalid_response")
		}
	case "oneapi":
		// Legacy billing can expose either account or key figures with a site-defined
		// unit. Require the explicit token endpoint rather than mislabeling it.
		if err := a.user(); err != nil {
			return nil, err
		}
		if id < 1 {
			return nil, failure("remote_key_required")
		}
		raw, err := a.get(ctx, "/api/token/"+remoteID(id), true, nil)
		if err != nil {
			return nil, err
		}
		m := data(raw)
		out.Remaining = amount(m, "remain_quota", "quota")
		out.Used = amount(m, "used_quota", "quota")
		out.Unlimited = boolean(m, "unlimited_quota")
		if out.Remaining == nil {
			return nil, failure("invalid_response")
		}
	default:
		return nil, failure("unsupported")
	}
	return out, nil
}

func (a *Adapter) GetEffectiveRate(ctx context.Context, id int64) (*Rate, error) {
	if a.Config.Platform == "oneapi" {
		return nil, failure("unsupported")
	}
	if err := a.user(); err != nil {
		return nil, err
	}
	if id < 1 {
		return nil, failure("remote_key_required")
	}
	path := "/api/token/"
	if a.Config.Platform == "sub2api" {
		path = "/api/v1/keys/"
	}
	raw, err := a.get(ctx, path+remoteID(id), true, nil)
	if err != nil {
		return nil, err
	}
	key := data(raw)
	if key == nil {
		return nil, failure("invalid_response")
	}
	if a.Config.Platform == "sub2api" {
		group := identifier(key, "group_id")
		if group == "" {
			return nil, failure("unknown")
		}
		out := &Rate{Group: group, GroupName: text(dataGroup(key), "name"), Basis: "authenticated_group"}
		raw, err = a.get(ctx, "/api/v1/groups/rates", true, nil)
		if err != nil {
			return out, err
		}
		// A null rates map is valid and means no personal overrides.
		rates := data(raw)
		if v, ok := raw["data"]; !ok || (string(v) != "null" && rates == nil) {
			return out, failure("invalid_response")
		}
		if v := number(rates, group); v != nil {
			out.Multiplier = v
			out.Basis = "personal_override"
			return out, nil
		}
		raw, err = a.get(ctx, "/api/v1/groups/available", true, nil)
		if err != nil {
			return out, err
		}
		groups, err := array(raw["data"])
		if err != nil {
			return out, err
		}
		for _, g := range groups {
			if identifier(g, "id") == group {
				out.Multiplier = number(g, "rate_multiplier")
				out.GroupName = text(g, "name")
				break
			}
		}
		if out.Multiplier == nil {
			return out, failure("unknown")
		}
		return out, nil
	}
	group := text(key, "group")
	if group == "" {
		raw, err = a.get(ctx, "/api/user/self", true, nil)
		if err != nil {
			return nil, err
		}
		group = text(data(raw), "group")
	}
	// Auto-group keys can switch group per request; there is no one current rate.
	out := &Rate{Group: group, Basis: "authenticated_group"}
	if group == "auto" || group == "" {
		return out, failure("unknown")
	}
	raw, err = a.get(ctx, "/api/pricing", true, nil)
	if err != nil {
		return out, err
	}
	var ratios object
	if json.Unmarshal(raw["group_ratio"], &ratios) != nil {
		return out, failure("invalid_response")
	}
	multiplier := number(ratios, group)
	if multiplier == nil {
		return out, failure("unknown")
	}
	out.Multiplier = multiplier
	return out, nil
}

func sub2KeyQuota(m object) (*Allowance, error) {
	out := allowance("key")
	switch text(m, "mode") {
	case "unrestricted":
		// This only says the key has no own cap. Wallet/subscription funds are separate.
		unlimited := true
		out.Unlimited = &unlimited
	case "quota_limited":
		if raw, ok := m["quota"]; ok {
			var q object
			if json.Unmarshal(raw, &q) != nil || q == nil {
				return nil, failure("invalid_response")
			}
			out.Remaining = amount(q, "remaining", "USD")
			out.Used = amount(q, "used", "USD")
			out.Limit = amount(q, "limit", "USD")
			if out.Remaining == nil || out.Used == nil || out.Limit == nil {
				return nil, failure("invalid_response")
			}
		}
		if raw, ok := m["rate_limits"]; ok {
			windows, err := array(raw)
			if err != nil {
				return nil, err
			}
			for _, w := range windows {
				v := Window{Period: text(w, "window"), Remaining: amount(w, "remaining", "USD"), Used: amount(w, "used", "USD"), Limit: amount(w, "limit", "USD"), ResetAt: text(w, "reset_at")}
				if v.Period == "" || v.Remaining == nil || v.Used == nil || v.Limit == nil {
					return nil, failure("invalid_response")
				}
				out.Windows = append(out.Windows, v)
			}
		}
		// A quota service can omit a window on transient lookup failure. Do not
		// interpret that omission as unlimited or a zero balance.
		if out.Limit == nil && len(out.Windows) == 0 {
			return nil, failure("unknown")
		}
	default:
		return nil, failure("invalid_response")
	}
	return out, nil
}
