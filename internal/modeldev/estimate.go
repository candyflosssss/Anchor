package modeldev

import (
	"math/big"
	"strings"
)

// Estimate is pure, so logging can use a captured catalog price without network
// I/O in the proxy path. Cache and reasoning tokens are never billed twice.
func Estimate(model CatalogModel, u CatalogUsage) (CatalogEstimate, error) {
	out := CatalogEstimate{Usage: u, Basis: "text_tokens", Currency: "USD"}
	if u.Validate() != nil {
		return out, ErrInvalid
	}
	if model.Pricing == nil {
		out.Reason = "missing_price"
		return out, nil
	}
	if u.InputTokens == nil || u.OutputTokens == nil {
		out.Reason = "missing_usage"
		return out, nil
	}
	if u.CacheReadTokens == nil || u.CacheWriteTokens == nil {
		out.Reason = "missing_cache_usage"
		return out, nil
	}
	rates := model.Pricing.Rates
	for _, tier := range model.Pricing.Tiers {
		if *u.InputTokens > tier.AboveTokens && (out.AboveTokens == nil || tier.AboveTokens > *out.AboveTokens) {
			rates = tier.Rates
			threshold := tier.AboveTokens
			out.AboveTokens = &threshold
		}
	}
	var reasoning int64
	if u.ReasoningTokens != nil {
		reasoning = *u.ReasoningTokens
	} else if rates.Reasoning != nil && (rates.Output == nil || !sameRate(*rates.Reasoning, *rates.Output)) {
		out.Reason = "missing_reasoning_usage"
		return out, nil
	}
	reasoningRate := rates.Reasoning
	if reasoningRate == nil {
		reasoningRate = rates.Output
	}
	parts := []struct {
		tokens int64
		rate   *string
		dst    **string
		reason string
	}{
		{*u.InputTokens - *u.CacheReadTokens - *u.CacheWriteTokens, rates.Input, &out.InputUSD, "missing_input_price"},
		{*u.OutputTokens - reasoning, rates.Output, &out.OutputUSD, "missing_output_price"},
		{*u.CacheReadTokens, rates.CacheRead, &out.CacheReadUSD, "missing_cache_read_price"},
		{*u.CacheWriteTokens, rates.CacheWrite, &out.CacheWriteUSD, "missing_cache_write_price"},
		{reasoning, reasoningRate, &out.ReasoningUSD, "missing_reasoning_price"},
	}
	total := new(big.Rat)
	for _, p := range parts {
		cost := new(big.Rat)
		if p.tokens > 0 {
			if p.rate == nil {
				out.Reason = p.reason
				return out, nil
			}
			rate, ok := parseRate(*p.rate)
			if !ok || rate.Sign() < 0 {
				return out, ErrInvalid
			}
			cost.Mul(rate, big.NewRat(p.tokens, 1000000))
		}
		value := decimal(cost)
		*p.dst = &value
		total.Add(total, cost)
	}
	usd := decimal(total)
	out.USD = &usd
	return out, nil
}
func sameRate(a, b string) bool {
	x, ok := parseRate(a)
	y, ok2 := parseRate(b)
	return ok && ok2 && x.Cmp(y) == 0
}
func decimal(v *big.Rat) string {
	s := strings.TrimRight(strings.TrimRight(v.FloatString(18), "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}
