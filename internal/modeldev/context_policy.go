package modeldev

// ContextPolicy is advice, not an upstream enforcement or billing guarantee.
// ModelContext is the advertised capacity. RecommendedContext stays within the
// first declared input-price band; the actual tier is selected per request by
// inclusive input usage (including cache), never by the configured window.
type ContextPolicy struct {
	ModelContext       *int64 `json:"model_context"`
	RecommendedContext *int64 `json:"recommended_context"`
	PriceThreshold     *int64 `json:"price_threshold"`
}

// ContextPolicyFor also works on old saved snapshots without derived metadata.
// It never guesses thresholds from model names or the lossy legacy field when
// exact tiers are present (the parser already normalizes that precedence).
func ContextPolicyFor(m CatalogModel) *ContextPolicy {
	p := &ContextPolicy{ModelContext: m.Limits.Context, RecommendedContext: m.Limits.Context}
	if m.Pricing != nil {
		for _, tier := range m.Pricing.Tiers {
			if tier.AboveTokens > 0 && (p.PriceThreshold == nil || tier.AboveTokens < *p.PriceThreshold) {
				n := tier.AboveTokens
				p.PriceThreshold = &n
			}
		}
	}
	if p.RecommendedContext != nil && p.PriceThreshold != nil && *p.PriceThreshold < *p.RecommendedContext {
		p.RecommendedContext = p.PriceThreshold
	}
	return p
}
