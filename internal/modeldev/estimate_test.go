package modeldev

import (
	"errors"
	"testing"
)

func n(v int64) *int64      { return &v }
func text(v string) *string { return &v }
func TestEstimateInclusiveCountsAndTiers(t *testing.T) {
	m, e := ParseModel([]byte(fixture(t)), "lab", "vision-reasoner")
	if e != nil {
		t.Fatal(e)
	}
	u := CatalogUsage{InputTokens: n(1000), OutputTokens: n(100), CacheReadTokens: n(400), CacheWriteTokens: n(100), ReasoningTokens: n(30)}
	out, e := Estimate(m, u)
	if e != nil || out.USD == nil || *out.USD != "0.00265" || *out.InputUSD != "0.00125" || *out.OutputUSD != "0.0007" || *out.ReasoningUSD != "0.0003" {
		t.Fatal(out, e)
	}
	u = CatalogUsage{InputTokens: n(250000), OutputTokens: n(0), CacheReadTokens: n(0), CacheWriteTokens: n(0)}
	out, e = Estimate(m, u)
	if e != nil || out.AboveTokens != nil || *out.USD != "0.625" {
		t.Fatal(out, e)
	}
	u.InputTokens = n(250001)
	out, e = Estimate(m, u)
	if e != nil || out.AboveTokens == nil || *out.AboveTokens != 250000 || *out.USD != "1.250005" {
		t.Fatal(out, e)
	}
	legacy, e := ParseModel([]byte(`{"p":{"models":{"m":{"cost":{"input":1,"output":2,"context_over_200k":{"input":3,"output":4}}}}}}`), "p", "m")
	if e != nil || legacy.Pricing.Tiers[0].AboveTokens != 200000 {
		t.Fatal(legacy, e)
	}
}
func TestEstimateUnknownFreeAndInvalid(t *testing.T) {
	m, _ := ParseModel([]byte(fixture(t)), "lab", "vision-reasoner")
	u := CatalogUsage{InputTokens: n(100), OutputTokens: n(50), CacheReadTokens: n(0), CacheWriteTokens: n(0)}
	for _, tc := range []struct {
		name   string
		model  CatalogModel
		usage  CatalogUsage
		reason string
	}{
		{"price", CatalogModel{}, u, "missing_price"},
		{"usage", m, CatalogUsage{}, "missing_usage"},
		{"cache", m, CatalogUsage{InputTokens: n(100), OutputTokens: n(50)}, "missing_cache_usage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, e := Estimate(tc.model, tc.usage)
			if e != nil || out.USD != nil || out.Reason != tc.reason {
				t.Fatal(out, e)
			}
		})
	}
	m.Pricing.Rates.Reasoning = text("20")
	out, e := Estimate(m, u)
	if e != nil || out.Reason != "missing_reasoning_usage" {
		t.Fatal(out, e)
	}
	u.ReasoningTokens = n(0)
	u.CacheReadTokens = n(1)
	m.Pricing.Rates.CacheRead = nil
	out, e = Estimate(m, u)
	if e != nil || out.USD != nil || out.Reason != "missing_cache_read_price" {
		t.Fatal(out, e)
	}
	free, _ := ParseModel([]byte(fixture(t)), "lab", "text-only")
	u.CacheReadTokens = n(0)
	out, e = Estimate(free, u)
	if e != nil || out.USD == nil || *out.USD != "0" {
		t.Fatal(out, e)
	}
	for _, u := range []CatalogUsage{{InputTokens: n(-1)}, {InputTokens: n(10), CacheReadTokens: n(8), CacheWriteTokens: n(3)}, {OutputTokens: n(10), ReasoningTokens: n(11)}, {InputTokens: n(MaxSafeInteger + 1)}} {
		if _, e := Estimate(m, u); !errors.Is(e, ErrInvalid) {
			t.Fatal(u, e)
		}
	}
	m.Pricing.Rates.Input = text("1e-999999999")
	u.CacheReadTokens = n(0)
	if _, e := Estimate(m, u); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
