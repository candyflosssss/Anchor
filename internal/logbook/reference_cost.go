package logbook

// CostPart is one disjoint text-token charging bucket. Rate is USD per million
// tokens; MixedRate marks an aggregate whose attempts selected different tiers.
type CostPart struct {
	Kind              string  `json:"kind"`
	Tokens            *int64  `json:"tokens"`
	Rate              *string `json:"rate"`
	RateMultiplier    *string `json:"rate_multiplier"`
	IncludesReasoning bool    `json:"includes_reasoning,omitempty"`
	MixedRate         bool    `json:"mixed_rate,omitempty"`
	USD               *string `json:"usd"`
	FinalUSD          *string `json:"final_usd"`
}

// CostAdjustment freezes the selected provider and Key's last successful rate
// observation before dispatch. A nil group multiplier is unknown, never one.
// On root rows the parts/totals are sums; mixed factors must not be applied to
// the root original cost as if they belonged to every attempt.
type CostAdjustment struct {
	USD                *string    `json:"usd"`
	Reason             string     `json:"reason"`
	SupplierMultiplier *string    `json:"supplier_multiplier"`
	GroupMultiplier    *string    `json:"group_multiplier"`
	SupplierMixed      bool       `json:"supplier_mixed,omitempty"`
	GroupMixed         bool       `json:"group_mixed,omitempty"`
	Group              string     `json:"group"`
	GroupName          string     `json:"group_name"`
	Basis              string     `json:"basis"`
	ObservedAt         string     `json:"observed_at"`
	Failure            string     `json:"failure"`
	Parts              []CostPart `json:"parts"`
}
