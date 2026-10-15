package modeldev

import (
	"encoding/json"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type rawRates struct {
	Input       *json.Number `json:"input"`
	Output      *json.Number `json:"output"`
	CacheRead   *json.Number `json:"cache_read"`
	CacheWrite  *json.Number `json:"cache_write"`
	Reasoning   *json.Number `json:"reasoning"`
	InputAudio  *json.Number `json:"input_audio"`
	OutputAudio *json.Number `json:"output_audio"`
}
type rawCost struct {
	rawRates
	Tiers []struct {
		rawRates
		Tier struct {
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"tier"`
	} `json:"tiers"`
	Legacy *rawRates `json:"context_over_200k"`
}
type rawModel struct {
	Name        string `json:"name"`
	Canonical   string `json:"canonical_model_id"`
	Updated     string `json:"last_updated"`
	Attachment  *bool  `json:"attachment"`
	Tools       *bool  `json:"tool_call"`
	Reasoning   *bool  `json:"reasoning"`
	Structured  *bool  `json:"structured_output"`
	Temperature *bool  `json:"temperature"`
	Modalities  *struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"modalities"`
	Limits  CatalogLimits     `json:"limit"`
	Options []ReasoningOption `json:"reasoning_options"`
	Cost    *rawCost          `json:"cost"`
}
type providerData struct {
	Info   CatalogProvider
	Models map[string]CatalogModel
}

func parse(raw []byte) (map[string]providerData, error) {
	var data map[string]struct {
		Name   string              `json:"name"`
		Models map[string]rawModel `json:"models"`
	}
	if len(raw) > MaxBytes || json.Unmarshal(raw, &data) != nil || len(data) == 0 || len(data) > 1024 {
		return nil, ErrInvalid
	}
	out := make(map[string]providerData, len(data))
	total := 0
	for pid, p := range data {
		p.Name = strings.TrimSpace(p.Name)
		if !validText(pid, 128, false) || !validText(p.Name, 256, true) || p.Models == nil {
			return nil, ErrInvalid
		}
		dst := providerData{Info: CatalogProvider{ID: pid, Name: p.Name, ModelCount: len(p.Models)}, Models: make(map[string]CatalogModel, len(p.Models))}
		for id, m := range p.Models {
			m.Name = strings.TrimSpace(m.Name)
			total++
			if total > 50000 || !validText(id, 256, false) || !validText(m.Name, 256, true) || !validText(m.Canonical, 256, true) || !validText(m.Updated, 32, true) {
				return nil, ErrInvalid
			}
			for _, n := range []*int64{m.Limits.Context, m.Limits.Input, m.Limits.Output} {
				if n != nil && (*n < 0 || *n > MaxSafeInteger) {
					return nil, ErrInvalid
				}
			}
			// A zero limit in the directory is not a usable model limit.
			limits := m.Limits
			if limits.Context != nil && *limits.Context == 0 {
				limits.Context = nil
			}
			if limits.Input != nil && *limits.Input == 0 {
				limits.Input = nil
			}
			if limits.Output != nil && *limits.Output == 0 {
				limits.Output = nil
			}
			caps := CatalogCapabilities{InputModalities: []string{}, OutputModalities: []string{}, Attachment: m.Attachment, ToolCall: m.Tools, Reasoning: m.Reasoning, StructuredOutput: m.Structured, Temperature: m.Temperature}
			if m.Modalities != nil {
				if len(m.Modalities.Input) > 16 || len(m.Modalities.Output) > 16 {
					return nil, ErrInvalid
				}
				for _, list := range [][]string{m.Modalities.Input, m.Modalities.Output} {
					for _, v := range list {
						if !validText(v, 32, false) {
							return nil, ErrInvalid
						}
					}
				}
				if m.Modalities.Input != nil {
					caps.InputModalities = m.Modalities.Input
				}
				if m.Modalities.Output != nil {
					caps.OutputModalities = m.Modalities.Output
				}
				if m.Modalities.Input != nil {
					vision := slices.Contains(caps.InputModalities, "image")
					caps.Vision = &vision
				}
			}
			if m.Options == nil {
				m.Options = []ReasoningOption{}
			}
			if len(m.Options) > 16 {
				return nil, ErrInvalid
			}
			for i := range m.Options {
				o := &m.Options[i]
				if o.Values == nil {
					o.Values = []*string{}
				}
				switch o.Type {
				case "toggle":
					if len(o.Values) != 0 || o.Min != nil || o.Max != nil {
						return nil, ErrInvalid
					}
				case "effort":
					if len(o.Values) == 0 || len(o.Values) > 16 || o.Min != nil || o.Max != nil {
						return nil, ErrInvalid
					}
					for _, v := range o.Values {
						if v != nil && !slices.Contains([]string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "default"}, *v) {
							return nil, ErrInvalid
						}
					}
				case "budget_tokens":
					if len(o.Values) != 0 || (o.Min != nil && (*o.Min < -1 || *o.Min > MaxSafeInteger)) || (o.Max != nil && (*o.Max < 0 || *o.Max > MaxSafeInteger)) || (o.Min != nil && o.Max != nil && *o.Min > *o.Max) {
						return nil, ErrInvalid
					}
				default:
					return nil, ErrInvalid
				}
			}
			if m.Reasoning != nil && !*m.Reasoning && len(m.Options) > 0 {
				return nil, ErrInvalid
			}
			var price *CatalogPricing
			if m.Cost != nil {
				rates, err := m.Cost.rawRates.convert()
				if err != nil {
					return nil, err
				}
				price = &CatalogPricing{Currency: "USD", Unit: "per_million_tokens", Rates: rates, Tiers: []CatalogTier{}}
				if len(m.Cost.Tiers) > 32 {
					return nil, ErrInvalid
				}
				for _, tier := range m.Cost.Tiers {
					if (tier.Tier.Type != "" && tier.Tier.Type != "context") || tier.Tier.Size < 0 || tier.Tier.Size > MaxSafeInteger {
						return nil, ErrInvalid
					}
					r, e := tier.rawRates.convert()
					if e != nil {
						return nil, e
					}
					price.Tiers = append(price.Tiers, CatalogTier{AboveTokens: tier.Tier.Size, Rates: r})
				}
				// Exact tiers supersede the lossy legacy compatibility field.
				if len(price.Tiers) == 0 && m.Cost.Legacy != nil {
					r, e := m.Cost.Legacy.convert()
					if e != nil {
						return nil, e
					}
					price.Tiers = append(price.Tiers, CatalogTier{AboveTokens: 200000, Rates: r})
				}
				slices.SortFunc(price.Tiers, func(a, b CatalogTier) int {
					if a.AboveTokens < b.AboveTokens {
						return -1
					}
					if a.AboveTokens > b.AboveTokens {
						return 1
					}
					return 0
				})
				for i := 1; i < len(price.Tiers); i++ {
					if price.Tiers[i].AboveTokens == price.Tiers[i-1].AboveTokens {
						return nil, ErrInvalid
					}
				}
			}
			model := CatalogModel{ProviderID: pid, ID: id, CanonicalModelID: m.Canonical, Name: m.Name, SourceUpdatedAt: m.Updated, Capabilities: caps, Limits: limits, ReasoningOptions: m.Options, Pricing: price}
			model.ContextPolicy = ContextPolicyFor(model)
			dst.Models[id] = model
		}
		out[pid] = dst
	}
	return out, nil
}
func (r rawRates) convert() (CatalogRates, error) {
	out := CatalogRates{}
	for _, pair := range []struct {
		src *json.Number
		dst **string
	}{{r.Input, &out.Input}, {r.Output, &out.Output}, {r.CacheRead, &out.CacheRead}, {r.CacheWrite, &out.CacheWrite}, {r.Reasoning, &out.Reasoning}, {r.InputAudio, &out.InputAudio}, {r.OutputAudio, &out.OutputAudio}} {
		if pair.src == nil {
			continue
		}
		s := pair.src.String()
		_, ok := parseRate(s)
		if !ok {
			return CatalogRates{}, ErrInvalid
		}
		*pair.dst = &s
	}
	return out, nil
}

// ParseModel shares the strict decoder with offline compatibility importers.
func ParseModel(raw []byte, providerID, modelID string) (CatalogModel, error) {
	if (ModelCatalogQuery{ProviderID: providerID, ModelID: modelID}).Validate() != nil || providerID == "" || modelID == "" {
		return CatalogModel{}, ErrInvalid
	}
	data, err := parse(raw)
	if err != nil {
		return CatalogModel{}, err
	}
	entry, ok := data[providerID].Models[modelID]
	if !ok {
		return CatalogModel{}, ErrNotFound
	}
	return entry, nil
}

var decimalRate = regexp.MustCompile(`^(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

// Bound numeric text before rational parsing: extreme exponents otherwise
// allocate unbounded powers of ten even in a tiny remote JSON document.
func parseRate(s string) (*big.Rat, bool) {
	if len(s) > 64 || !decimalRate.MatchString(s) {
		return nil, false
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		e, err := strconv.Atoi(s[i+1:])
		if err != nil || e < -308 || e > 308 {
			return nil, false
		}
	}
	v, ok := new(big.Rat).SetString(s)
	return v, ok && v.Sign() >= 0 && v.Cmp(big.NewRat(1000000, 1)) <= 0
}
