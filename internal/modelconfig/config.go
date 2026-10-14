// Package modelconfig contains portable, validated model declarations.
package modelconfig

import (
	"errors"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/candyflosssss/Anchor/internal/modeldev"
)

// Reasoning defaults retain the provider's native representation. A nil Value
// for effort means the native JSON null option, not the string "none".
type ReasoningDefault struct {
	Type         string  `json:"type"`
	Value        *string `json:"value"`
	BudgetTokens *int64  `json:"budget_tokens"`
	Enabled      *bool   `json:"enabled"`
}

type ModelConfiguration struct {
	Limits              modeldev.CatalogLimits       `json:"limits"`
	Capabilities        modeldev.CatalogCapabilities `json:"capabilities"`
	Streaming           *bool                        `json:"streaming"`
	ParallelToolCalls   *bool                        `json:"parallel_tool_calls"`
	SupportedReasoning  []modeldev.ReasoningOption   `json:"supported_reasoning"`
	DefaultReasoning    *ReasoningDefault            `json:"default_reasoning"`
	DefaultOutputTokens *int64                       `json:"default_output_tokens"`
	DefaultTemperature  *float64                     `json:"default_temperature"`
}

type ModelSettings struct {
	Configuration ModelConfiguration     `json:"configuration"`
	Reference     *modeldev.ModelCatalog `json:"reference"`
}

func validSettingText(v string) bool {
	return v != "" && len(v) <= 128 && utf8.ValidString(v) && strings.TrimSpace(v) == v && !strings.ContainsFunc(v, unicode.IsControl)
}

func (c ModelConfiguration) Validate() error {
	invalid := errors.New("invalid model configuration")
	for _, n := range []*int64{c.Limits.Context, c.Limits.Input, c.Limits.Output, c.DefaultOutputTokens} {
		if n != nil && (*n < 1 || *n > modeldev.MaxSafeInteger) {
			return invalid
		}
	}
	if c.Limits.Context != nil {
		for _, n := range []*int64{c.Limits.Input, c.Limits.Output, c.DefaultOutputTokens} {
			if n != nil && *n > *c.Limits.Context {
				return invalid
			}
		}
	}
	if c.DefaultOutputTokens != nil && c.Limits.Output != nil && *c.DefaultOutputTokens > *c.Limits.Output {
		return errors.New("default output exceeds model maximum")
	}
	if c.DefaultTemperature != nil {
		n := *c.DefaultTemperature
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 2 || c.Capabilities.Temperature == nil || !*c.Capabilities.Temperature {
			return errors.New("temperature default requires declared support and range 0..2")
		}
	}
	if c.ParallelToolCalls != nil && *c.ParallelToolCalls && c.Capabilities.ToolCall != nil && !*c.Capabilities.ToolCall {
		return invalid
	}
	for _, list := range [][]string{c.Capabilities.InputModalities, c.Capabilities.OutputModalities} {
		if len(list) > 16 {
			return invalid
		}
		seen := map[string]bool{}
		for _, v := range list {
			if !validSettingText(v) || seen[v] {
				return invalid
			}
			seen[v] = true
		}
	}
	if c.Capabilities.Vision != nil && len(c.Capabilities.InputModalities) > 0 {
		image := false
		for _, v := range c.Capabilities.InputModalities {
			if v == "image" {
				image = true
			}
		}
		if *c.Capabilities.Vision != image {
			return errors.New("vision and input modalities disagree")
		}
	}
	if len(c.SupportedReasoning) > 3 {
		return invalid
	}
	seen := map[string]bool{}
	for _, o := range c.SupportedReasoning {
		if seen[o.Type] {
			return invalid
		}
		seen[o.Type] = true
		switch o.Type {
		case "effort":
			if len(o.Values) < 1 || len(o.Values) > 32 || o.Min != nil || o.Max != nil {
				return invalid
			}
			values := map[string]bool{}
			nullSeen := false
			for _, v := range o.Values {
				if v == nil {
					if nullSeen {
						return invalid
					}
					nullSeen = true
				} else {
					if !validSettingText(*v) || values[*v] {
						return invalid
					}
					values[*v] = true
				}
			}
		case "toggle":
			if len(o.Values) > 0 || o.Min != nil || o.Max != nil {
				return invalid
			}
		case "budget_tokens":
			if len(o.Values) > 0 {
				return invalid
			}
			for _, n := range []*int64{o.Min, o.Max} {
				if n != nil && (*n < -1 || *n > modeldev.MaxSafeInteger) {
					return invalid
				}
			}
			if o.Max != nil && *o.Max < 0 {
				return invalid
			}
			if o.Min != nil && o.Max != nil && *o.Min > *o.Max {
				return invalid
			}
		default:
			return invalid
		}
	}
	if c.Capabilities.Reasoning != nil && !*c.Capabilities.Reasoning && (len(c.SupportedReasoning) > 0 || c.DefaultReasoning != nil) {
		return errors.New("reasoning disabled but options/default supplied")
	}
	if d := c.DefaultReasoning; d != nil {
		if !seen[d.Type] {
			return errors.New("default reasoning requires a supported option")
		}
		for _, o := range c.SupportedReasoning {
			if o.Type != d.Type {
				continue
			}
			switch d.Type {
			case "effort":
				if d.BudgetTokens != nil || d.Enabled != nil {
					return invalid
				}
				found := false
				for _, v := range o.Values {
					if (v == nil && d.Value == nil) || (v != nil && d.Value != nil && *v == *d.Value) {
						found = true
					}
				}
				if !found {
					return errors.New("default reasoning effort is unsupported")
				}
			case "toggle":
				if d.Enabled == nil || d.Value != nil || d.BudgetTokens != nil {
					return invalid
				}
			case "budget_tokens":
				if d.BudgetTokens == nil || d.Value != nil || d.Enabled != nil {
					return invalid
				}
				n := *d.BudgetTokens
				if n < -1 || n > modeldev.MaxSafeInteger || (o.Min != nil && n < *o.Min) || (o.Max != nil && n > *o.Max) {
					return errors.New("default reasoning budget is unsupported")
				}
			}
		}
	}
	return nil
}

func (s ModelSettings) Validate() error {
	if err := s.Configuration.Validate(); err != nil {
		return err
	}
	if r := s.Reference; r != nil {
		if r.Source != "models.dev" || len(r.Models) != 1 || len(r.Providers) != 0 || len(r.Version) != 64 || r.FetchedAt < 1 || r.ExpiresAt < r.FetchedAt || r.Models[0].ProviderID == "" || r.Models[0].ID == "" {
			return errors.New("invalid reference snapshot")
		}
	}
	return nil
}
