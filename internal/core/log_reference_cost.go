package core

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
)

func referenceCost(snapshot *modeldev.ModelCatalog) *logbook.ReferenceCost {
	if snapshot == nil || len(snapshot.Models) != 1 || snapshot.Models[0].Pricing == nil {
		return nil
	}
	m := snapshot.Models[0]
	stale := snapshot.Stale || (snapshot.FetchedAt > 0 && time.Since(time.Unix(snapshot.FetchedAt, 0)) >= modeldev.CacheTTL) || (snapshot.ExpiresAt > 0 && time.Now().Unix() >= snapshot.ExpiresAt)
	return &logbook.ReferenceCost{Currency: "USD", Basis: "text_tokens", Reason: "missing_usage", Source: snapshot.Source, ProviderID: m.ProviderID, ModelID: m.ID, CatalogVersion: snapshot.Version, FetchedAt: snapshot.FetchedAt, Stale: stale, Pricing: m.Pricing}
}

func (s *Server) cachedLogReference(model string) *logbook.ReferenceCost {
	if s.modelCatalog == nil {
		return nil
	}
	snapshot, err := s.modelCatalog.ResolveCached(model)
	if err != nil {
		return nil
	}
	return referenceCost(&snapshot)
}

func (s *Server) logReference(m state.Model) *logbook.ReferenceCost {
	if m.Settings != nil && m.Settings.Reference != nil {
		// The saved reference chooses an identity, not an indefinitely frozen
		// price. Prefer that identity's current cache without changing settings.
		saved := m.Settings.Reference
		if len(saved.Models) != 1 {
			return nil
		}
		model := saved.Models[0]
		if price := s.cachedLogReference(model.ProviderID + "/" + model.ID); price != nil {
			return price
		}
		if saved.FetchedAt <= 0 || time.Since(time.Unix(saved.FetchedAt, 0)) >= modeldev.ReferenceTTL {
			return nil
		}
		return referenceCost(saved)
	}
	// A speed variant has no inferred standard reference price. An explicitly
	// selected reference above is still a reference, never a service-tier bill.
	if m.ServiceTier != "" || len(m.Candidates) == 0 {
		return nil
	}
	var selected *logbook.ReferenceCost
	for _, candidate := range m.Candidates {
		price := s.cachedLogReference(candidate.Model)
		if price == nil {
			return nil
		}
		if selected != nil && (selected.ProviderID != price.ProviderID || selected.ModelID != price.ModelID || selected.CatalogVersion != price.CatalogVersion) {
			return nil
		}
		selected = price
	}
	return selected
}

func logEstimate(price *logbook.ReferenceCost, u protocol.Usage) modeldev.CatalogEstimate {
	usage := modeldev.CatalogUsage{}
	if u.Known {
		usage.InputTokens, usage.OutputTokens = &u.Input, &u.Output
	}
	if u.CacheReadKnown {
		usage.CacheReadTokens = &u.CacheRead
	}
	if u.CacheWriteKnown {
		usage.CacheWriteTokens = &u.CacheWrite
	}
	if u.ReasoningKnown {
		usage.ReasoningTokens = &u.Reasoning
	}
	estimate, err := modeldev.Estimate(modeldev.CatalogModel{Pricing: price.Pricing}, usage)
	if err != nil {
		return modeldev.CatalogEstimate{Reason: "invalid_usage", Usage: usage}
	}
	return estimate
}

func estimateLogAttempt(price *logbook.ReferenceCost, u protocol.Usage) (*string, string) {
	e := logEstimate(price, u)
	return e.USD, e.Reason
}

func logCostFactors(p state.Provider, key state.ProviderKey) *logbook.CostAdjustment {
	supplier := p.CostMultiplier
	if supplier == 0 {
		supplier = 1
	} // persisted legacy provider default
	out := &logbook.CostAdjustment{SupplierMultiplier: logMultiplier(supplier), Parts: []logbook.CostPart{}}
	if key.Rate != nil {
		r := key.Rate
		out.Group, out.GroupName, out.Basis, out.ObservedAt, out.Failure = r.Group, r.GroupName, r.Basis, r.ObservedAt, r.Failure
		if r.UsableAt(time.Now()) {
			out.GroupMultiplier = logMultiplier(*r.Multiplier)
		}
	}
	return out
}
func logMultiplier(v float64) *string {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil
	}
	s := strconv.FormatFloat(v, 'f', -1, 64)
	return &s
}
func adjustedUSD(usd *string, a *logbook.CostAdjustment) *string {
	if usd == nil || a.SupplierMultiplier == nil || a.GroupMultiplier == nil {
		return nil
	}
	v, ok := new(big.Rat).SetString(*usd)
	if !ok {
		return nil
	}
	for _, factor := range []*string{a.SupplierMultiplier, a.GroupMultiplier} {
		r, ok := new(big.Rat).SetString(*factor)
		if !ok || r.Sign() < 0 {
			return nil
		}
		v.Mul(v, r)
	}
	s := strings.TrimRight(strings.TrimRight(v.FloatString(30), "0"), ".")
	if s == "" {
		s = "0"
	}
	return &s
}
func addCost(a, b *string) *string {
	if a == nil || b == nil {
		return nil
	}
	x, ok := new(big.Rat).SetString(*a)
	y, ok2 := new(big.Rat).SetString(*b)
	if !ok || !ok2 {
		return nil
	}
	s := strings.TrimRight(strings.TrimRight(x.Add(x, y).FloatString(30), "0"), ".")
	if s == "" {
		s = "0"
	}
	return &s
}
func sameCost(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
func priceAdjustment(price *logbook.ReferenceCost, a *logbook.CostAdjustment, e modeldev.CatalogEstimate) {
	a.USD = adjustedUSD(e.USD, a)
	a.Reason = e.Reason
	if a.Reason == "" && a.USD == nil {
		a.Reason = "missing_multiplier"
	}
	rates := price.Pricing.Rates
	threshold := int64(-1)
	for _, tier := range price.Pricing.Tiers {
		if e.Usage.InputTokens != nil && *e.Usage.InputTokens > tier.AboveTokens && tier.AboveTokens > threshold {
			rates, threshold = tier.Rates, tier.AboveTokens
		}
	}
	reasoningRate := rates.Reasoning
	if reasoningRate == nil {
		reasoningRate = rates.Output
	}
	u := e.Usage
	var input, output *int64
	if u.InputTokens != nil && u.CacheReadTokens != nil && u.CacheWriteTokens != nil {
		v := *u.InputTokens - *u.CacheReadTokens - *u.CacheWriteTokens
		if v >= 0 {
			input = &v
		}
	}
	reasoning := u.ReasoningTokens
	// Equal output/reasoning rates permit an inclusive output estimate, but
	// the unreported reasoning token count must remain unknown, not zero.
	inclusiveOutput := reasoning == nil && e.USD != nil
	if u.OutputTokens != nil {
		if reasoning != nil {
			v := *u.OutputTokens - *reasoning
			if v >= 0 {
				output = &v
			}
		} else if inclusiveOutput {
			output = u.OutputTokens
		}
	}
	a.Parts = []logbook.CostPart{
		{Kind: "input", Tokens: input, Rate: rates.Input, USD: e.InputUSD},
		{Kind: "cache_read", Tokens: u.CacheReadTokens, Rate: rates.CacheRead, USD: e.CacheReadUSD},
		{Kind: "cache_write", Tokens: u.CacheWriteTokens, Rate: rates.CacheWrite, USD: e.CacheWriteUSD},
		{Kind: "output", IncludesReasoning: inclusiveOutput, Tokens: output, Rate: rates.Output, USD: e.OutputUSD},
		{Kind: "reasoning", Tokens: reasoning, Rate: reasoningRate, USD: e.ReasoningUSD},
	}
	for i := range a.Parts {
		p := &a.Parts[i]
		if p.Rate != nil && rates.Input != nil {
			x, ok := new(big.Rat).SetString(*p.Rate)
			base, ok2 := new(big.Rat).SetString(*rates.Input)
			if ok && ok2 && base.Sign() > 0 {
				v := strings.TrimRight(strings.TrimRight(x.Quo(x, base).FloatString(12), "0"), ".")
				if v == "" {
					v = "0"
				}
				p.RateMultiplier = &v
			}
		}
		a.Parts[i].FinalUSD = adjustedUSD(a.Parts[i].USD, a)
	}
}
func accumulateAdjustment(price *logbook.ReferenceCost, a *logbook.CostAdjustment) {
	if price.Adjustment == nil {
		copy := *a
		copy.Parts = append([]logbook.CostPart(nil), a.Parts...)
		price.Adjustment = &copy
		return
	}
	total := price.Adjustment
	total.USD = addCost(total.USD, a.USD)
	if total.Reason == "" {
		total.Reason = a.Reason
	}
	if !sameCost(total.SupplierMultiplier, a.SupplierMultiplier) {
		total.SupplierMultiplier, total.SupplierMixed = nil, true
	}
	if !sameCost(total.GroupMultiplier, a.GroupMultiplier) || total.Group != a.Group || total.Basis != a.Basis || total.ObservedAt != a.ObservedAt || total.Failure != a.Failure || total.GroupName != a.GroupName {
		total.GroupMultiplier, total.GroupMixed = nil, true
		total.Group, total.GroupName, total.Basis, total.ObservedAt, total.Failure = "", "", "", "", ""
	}
	for i := range total.Parts {
		p, q := &total.Parts[i], a.Parts[i]
		p.IncludesReasoning = p.IncludesReasoning || q.IncludesReasoning
		p.USD, p.FinalUSD = addCost(p.USD, q.USD), addCost(p.FinalUSD, q.FinalUSD)
		if p.Tokens == nil || q.Tokens == nil {
			p.Tokens = nil
		} else {
			v := *p.Tokens + *q.Tokens
			p.Tokens = &v
		}
		if !sameCost(p.Rate, q.Rate) || !sameCost(p.RateMultiplier, q.RateMultiplier) {
			p.Rate, p.RateMultiplier, p.MixedRate = nil, nil, true
		}
	}
}

func (t *requestTrace) priceAttempt(a *logbook.Attempt, u protocol.Usage) {
	price := t.row.ReferenceCost
	if price == nil {
		a.CostAdjustment = nil
		return
	}
	e := logEstimate(price, u)
	usd, reason := e.USD, e.Reason
	if a.CostAdjustment == nil {
		a.CostAdjustment = &logbook.CostAdjustment{}
	}
	priceAdjustment(price, a.CostAdjustment, e)
	accumulateAdjustment(price, a.CostAdjustment)
	a.ReferenceUSD = usd
	if usd == nil {
		t.unknownCost = true
		price.USD, price.Reason = nil, reason
		return
	}
	if t.unknownCost {
		return
	}
	total := new(big.Rat)
	if price.USD != nil {
		total.SetString(*price.USD)
	}
	amount, _ := new(big.Rat).SetString(*usd)
	value := total.Add(total, amount).FloatString(18)
	price.USD, price.Reason = &value, ""
}
