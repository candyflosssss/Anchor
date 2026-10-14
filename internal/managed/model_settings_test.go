package managed

import (
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
	"reflect"
	"testing"
)

func settingsPtr[T any](v T) *T { return &v }
func TestCatalogAppliesPortableSettingsAndPreservesInstructions(t *testing.T) {
	model := state.Model{ID: "mapped", DisplayName: "Mapped", Codex: metadata(t), Settings: &state.ModelSettings{Configuration: state.ModelConfiguration{
		Limits:             modeldev.CatalogLimits{Context: settingsPtr(int64(272000))},
		Capabilities:       modeldev.CatalogCapabilities{Vision: settingsPtr(true)},
		SupportedReasoning: []modeldev.ReasoningOption{{Type: "effort", Values: []*string{settingsPtr("low"), settingsPtr("high")}}},
		DefaultReasoning:   &state.ReasoningDefault{Type: "effort", Value: settingsPtr("high")},
		ParallelToolCalls:  settingsPtr(false),
	}}}
	raw, _, err := buildCatalog([]state.Model{model}, "0.159.3")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Models []map[string]any `json:"models"`
	}
	if json.Unmarshal(raw, &result) != nil {
		t.Fatal(string(raw))
	}
	var original map[string]any
	json.Unmarshal(model.Codex.Entry, &original)
	entry := result.Models[0]
	if entry["context_window"] != float64(272000) || entry["default_reasoning_level"] != "high" || entry["supports_parallel_tool_calls"] != false || !reflect.DeepEqual(entry["input_modalities"], []any{"text", "image"}) {
		t.Fatal(entry)
	}
	for _, key := range []string{"base_instructions", "model_messages", "truncation_policy", "experimental_supported_tools"} {
		if !reflect.DeepEqual(entry[key], original[key]) {
			t.Fatal("changed template", key)
		}
	}
	model.Settings.Configuration.SupportedReasoning[0].Values = []*string{settingsPtr("max")}
	model.Settings.Configuration.DefaultReasoning = nil
	if _, _, err = buildCatalog([]state.Model{model}, "0.159.3"); err == nil {
		t.Fatal("unrepresentable effort exported")
	}
	model.Settings.Configuration.SupportedReasoning = nil
	model.Settings.Configuration.Capabilities.Reasoning = settingsPtr(false)
	raw, _, err = buildCatalog([]state.Model{model}, "0.159.3")
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(raw, &result)
	if len(result.Models[0]["supported_reasoning_levels"].([]any)) != 0 || result.Models[0]["default_reasoning_level"] != nil {
		t.Fatal(result)
	}
}
