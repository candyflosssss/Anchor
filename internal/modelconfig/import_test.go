package modelconfig

import (
	"testing"

	"github.com/candyflosssss/Anchor/internal/modeldev"
)

func TestFillCatalogPreservesManualConfiguration(t *testing.T) {
	context, input, output := int64(400000), int64(300000), int64(100000)
	manual := int64(500000)
	no, yes := false, true
	m := modeldev.CatalogModel{Limits: modeldev.CatalogLimits{Context: &context, Input: &input, Output: &output}, Capabilities: modeldev.CatalogCapabilities{Vision: &yes, Reasoning: &yes, InputModalities: []string{"text", "image"}}, ReasoningOptions: []modeldev.ReasoningOption{{Type: "toggle"}}}
	cfg := FillCatalog(ModelConfiguration{Limits: modeldev.CatalogLimits{Context: &manual}, Capabilities: modeldev.CatalogCapabilities{Vision: &no, Reasoning: &no}}, m)
	if *cfg.Limits.Context != manual || *cfg.Limits.Input != input || *cfg.Limits.Output != output || *cfg.Capabilities.Vision || len(cfg.SupportedReasoning) != 0 || cfg.DefaultReasoning != nil {
		t.Fatal(cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Capabilities.InputModalities) != 1 || len(m.Capabilities.InputModalities) != 2 {
		t.Fatal("mutated or inconsistent modalities")
	}
}

func TestFillCatalogSkipsConflictingLimits(t *testing.T) {
	context, input, output, budget := int64(100), int64(200), int64(50), int64(70)
	cfg := FillCatalog(ModelConfiguration{Limits: modeldev.CatalogLimits{Context: &context}, DefaultOutputTokens: &budget}, modeldev.CatalogModel{Limits: modeldev.CatalogLimits{Input: &input, Output: &output}})
	if cfg.Limits.Input != nil || cfg.Limits.Output != nil || *cfg.Limits.Context != 100 || *cfg.DefaultOutputTokens != 70 {
		t.Fatal(cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg = FillCatalog(ModelConfiguration{Limits: modeldev.CatalogLimits{Input: &input}}, modeldev.CatalogModel{Limits: modeldev.CatalogLimits{Context: &context}})
	if cfg.Limits.Context != nil || *cfg.Limits.Input != 200 {
		t.Fatal(cfg)
	}
}

func TestFillCatalogPreservesExplicitRelatedDeclarations(t *testing.T) {
	no, yes := false, true
	cfg := ModelConfiguration{Capabilities: modeldev.CatalogCapabilities{InputModalities: []string{"text", "image"}}, SupportedReasoning: []modeldev.ReasoningOption{{Type: "toggle"}}, ParallelToolCalls: &yes}
	out := FillCatalog(cfg, modeldev.CatalogModel{Capabilities: modeldev.CatalogCapabilities{Vision: &no, Reasoning: &no, ToolCall: &no}})
	if err := out.Validate(); err != nil {
		t.Fatal(err)
	}
	if !*out.Capabilities.Vision || !*out.Capabilities.Reasoning || !*out.Capabilities.ToolCall || len(out.Capabilities.InputModalities) != 2 || len(out.SupportedReasoning) != 1 {
		t.Fatal(out)
	}
}

func TestFillCatalogUsesPriceBandWithoutChangingModelCapacity(t *testing.T) {
	maxContext, input, output, manual := int64(1050000), int64(922000), int64(128000), int64(500000)
	m := modeldev.CatalogModel{Limits: modeldev.CatalogLimits{Context: &maxContext, Input: &input, Output: &output}, Pricing: &modeldev.CatalogPricing{Tiers: []modeldev.CatalogTier{{AboveTokens: 272000}}}}
	cfg := FillCatalog(ModelConfiguration{}, m)
	if cfg.Validate() != nil || *cfg.Limits.Context != 272000 || *cfg.Limits.Input != 272000 || *cfg.Limits.Output != 128000 || *m.Limits.Context != 1050000 {
		t.Fatal(cfg, m)
	}
	cfg = FillCatalog(ModelConfiguration{Limits: modeldev.CatalogLimits{Context: &manual}}, m)
	if cfg.Validate() != nil || *cfg.Limits.Context != 500000 || cfg.Limits.Input != nil {
		t.Fatal("overwrote explicit window", cfg)
	}
	// An explicit input budget is never silently cut to the price band.
	cfg = FillCatalog(ModelConfiguration{Limits: modeldev.CatalogLimits{Input: &manual}}, m)
	if cfg.Validate() != nil || *cfg.Limits.Context != 1050000 || *cfg.Limits.Input != 500000 {
		t.Fatal("overwrote explicit input", cfg)
	}
}
