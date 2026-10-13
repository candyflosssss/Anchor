package providerapi

import (
	"context"
	"errors"
	"math"
	"strconv"
)

const sub2KeyBillingPath = "/v1/sub2api/billing"

func sub2KeyBillingSchema(m object) bool {
	version := integer(m, "schema_version")
	return text(m, "object") == "sub2api.key_billing" && version != nil && *version == 1 && text(m, "billing_scope") == "token"
}

// getSub2KeyBilling uses only the inference key. Only an absent endpoint can
// fall back to legacy account authorization; malformed billing or rejected
// credentials must remain visible. The server resolves user and peak rules.
func (a *Adapter) getSub2KeyBilling(ctx context.Context) (*Rate, bool, error) {
	m := a.keyBilling
	if m == nil {
		var err error
		m, err = a.get(ctx, sub2KeyBillingPath, false, nil)
		if err != nil {
			var f *Failure
			if errors.As(err, &f) && (f.Code == "unsupported" || (f.Code == "invalid_response" && a.htmlResponse)) {
				return nil, false, nil
			}
			return nil, true, err
		}
	}
	if !sub2KeyBillingSchema(m) || boolean(m, "peak_rate_enabled") == nil {
		return nil, true, failure("invalid_response")
	}
	for _, field := range []string{"group_rate_multiplier", "resolved_rate_multiplier", "effective_rate_multiplier"} {
		n := number(m, field)
		if n == nil {
			return nil, true, failure("invalid_response")
		}
		v, err := strconv.ParseFloat(n.String(), 64)
		if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
			return nil, true, failure("invalid_response")
		}
	}
	a.keyBilling = m
	// This endpoint does not disclose a group identifier or name.
	return &Rate{Multiplier: number(m, "effective_rate_multiplier"), Basis: "key_billing"}, true, nil
}
