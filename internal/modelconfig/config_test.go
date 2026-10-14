package modelconfig

import (
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"testing"
)

func p[T any](v T) *T { return &v }
func TestReasoningDefaultsAndLimits(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  ModelConfiguration
		bad  bool
	}{
		{"legacy unknown", ModelConfiguration{}, false},
		{"272k", ModelConfiguration{Limits: modeldev.CatalogLimits{Context: p(int64(272000)), Output: p(int64(32000))}, DefaultOutputTokens: p(int64(8000))}, false},
		{"output maximum", ModelConfiguration{Limits: modeldev.CatalogLimits{Output: p(int64(32))}, DefaultOutputTokens: p(int64(33))}, true},
		{"invalid context", ModelConfiguration{Limits: modeldev.CatalogLimits{Context: p(int64(0))}}, true},
		{"default without support", ModelConfiguration{DefaultReasoning: &ReasoningDefault{Type: "effort", Value: p("high")}}, true},
		{"native two levels", ModelConfiguration{SupportedReasoning: []modeldev.ReasoningOption{{Type: "effort", Values: []*string{p("low"), p("high")}}}, DefaultReasoning: &ReasoningDefault{Type: "effort", Value: p("high")}}, false},
		{"medium unsupported", ModelConfiguration{SupportedReasoning: []modeldev.ReasoningOption{{Type: "effort", Values: []*string{p("low"), p("high")}}}, DefaultReasoning: &ReasoningDefault{Type: "effort", Value: p("medium")}}, true},
		{"null effort", ModelConfiguration{SupportedReasoning: []modeldev.ReasoningOption{{Type: "effort", Values: []*string{nil}}}, DefaultReasoning: &ReasoningDefault{Type: "effort"}}, false},
		{"budget", ModelConfiguration{SupportedReasoning: []modeldev.ReasoningOption{{Type: "budget_tokens", Min: p(int64(1024)), Max: p(int64(4096))}}, DefaultReasoning: &ReasoningDefault{Type: "budget_tokens", BudgetTokens: p(int64(2048))}}, false},
		{"budget too small", ModelConfiguration{SupportedReasoning: []modeldev.ReasoningOption{{Type: "budget_tokens", Min: p(int64(1024))}}, DefaultReasoning: &ReasoningDefault{Type: "budget_tokens", BudgetTokens: p(int64(512))}}, true},
		{"toggle", ModelConfiguration{SupportedReasoning: []modeldev.ReasoningOption{{Type: "toggle"}}, DefaultReasoning: &ReasoningDefault{Type: "toggle", Enabled: p(false)}}, false},
		{"disabled contradiction", ModelConfiguration{Capabilities: modeldev.CatalogCapabilities{Reasoning: p(false)}, SupportedReasoning: []modeldev.ReasoningOption{{Type: "toggle"}}}, true},
		{"vision contradiction", ModelConfiguration{Capabilities: modeldev.CatalogCapabilities{Vision: p(false), InputModalities: []string{"text", "image"}}}, true},
		{"temperature without support", ModelConfiguration{DefaultTemperature: p(.5)}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); (err != nil) != tc.bad {
				t.Fatalf("err=%v bad=%v", err, tc.bad)
			}
		})
	}
}
