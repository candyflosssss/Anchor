package modeldev

import (
	"encoding/json"
	"os"
	"testing"
)

func contextPricingFixture(t *testing.T) CatalogModel {
	t.Helper()
	raw, err := os.ReadFile("../../desktop/contract/context-pricing.json")
	if err != nil {
		t.Fatal(err)
	}
	var m CatalogModel
	if err = json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestContextPolicySeparatesCapacityAndPriceBand(t *testing.T) {
	m := contextPricingFixture(t)
	p := ContextPolicyFor(m)
	if *p.ModelContext != 1050000 || *p.RecommendedContext != 272000 || *p.PriceThreshold != 272000 {
		t.Fatal(p)
	}
	m.ContextPolicy = nil // pre-policy pinned snapshots are recomputed
	if *ContextPolicyFor(m).RecommendedContext != 272000 {
		t.Fatal("old snapshot lost tier")
	}
	m.Limits.Context = n(128000)
	if *ContextPolicyFor(m).RecommendedContext != 128000 {
		t.Fatal("exceeded model capacity")
	}
	m.Limits.Context = nil
	if ContextPolicyFor(m).RecommendedContext != nil {
		t.Fatal("invented capacity from pricing")
	}
	m.Limits.Context = n(1050000)
	m.Pricing.Tiers = []CatalogTier{{AboveTokens: 500000}, {AboveTokens: 200000}, {AboveTokens: 272000}}
	if *ContextPolicyFor(m).RecommendedContext != 200000 {
		t.Fatal("used model-specific hardcode or tier order")
	}
	m.Pricing = nil
	p = ContextPolicyFor(m)
	if p.PriceThreshold != nil || *p.RecommendedContext != 1050000 {
		t.Fatal("invented price threshold")
	}
}

func TestLongContextWholeRequestPricingBoundaries(t *testing.T) {
	m := contextPricingFixture(t)
	for _, tc := range []struct {
		input, output, cache, write int64
		want, inputUSD, outputUSD   string
		tier                        bool
	}{
		{272000, 1000, 0, 0, "0.554", "0.544", "0.01", false},
		{272001, 1000, 0, 0, "1.103004", "1.088004", "0.015", true},
		{500000, 1000, 0, 0, "2.015", "2", "0.015", true},
		// Cached input counts toward the threshold, but is not charged twice.
		{272001, 1000, 272000, 0, "0.069404", "0.000004", "0.015", true},
		{272001, 1000, 0, 272000, "1.375004", "0.000004", "0.015", true},
		// Output size does not select an input-price band.
		{272000, 128000, 0, 0, "1.824", "0.544", "1.28", false},
	} {
		u := CatalogUsage{InputTokens: n(tc.input), OutputTokens: n(tc.output), CacheReadTokens: n(tc.cache), CacheWriteTokens: n(tc.write)}
		got, err := Estimate(m, u)
		if err != nil || got.USD == nil || *got.USD != tc.want || *got.InputUSD != tc.inputUSD || *got.OutputUSD != tc.outputUSD || (got.AboveTokens != nil) != tc.tier {
			t.Fatalf("usage=%+v estimate=%+v error=%v", u, got, err)
		}
	}
}
