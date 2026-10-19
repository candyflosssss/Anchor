package core

import (
	"math/big"
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
	return &logbook.ReferenceCost{Currency: "USD", Basis: "text_tokens", Reason: "missing_usage", Source: snapshot.Source, ProviderID: m.ProviderID, ModelID: m.ID, CatalogVersion: snapshot.Version, FetchedAt: snapshot.FetchedAt, Stale: snapshot.Stale || (snapshot.ExpiresAt > 0 && time.Now().Unix() >= snapshot.ExpiresAt), Pricing: m.Pricing}
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
		return referenceCost(m.Settings.Reference)
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

func estimateLogAttempt(price *logbook.ReferenceCost, u protocol.Usage) (*string, string) {
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
		return nil, "invalid_usage"
	}
	return estimate.USD, estimate.Reason
}

func (t *requestTrace) priceAttempt(a *logbook.Attempt, u protocol.Usage) {
	price := t.row.ReferenceCost
	if price == nil {
		return
	}
	usd, reason := estimateLogAttempt(price, u)
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
