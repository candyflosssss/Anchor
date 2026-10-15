package managed

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	raw, _, err = buildCatalog([]state.Model{model}, "0.159.3")
	if err != nil {
		t.Fatal("upstream-only effort blocked catalog", err)
	}
	json.Unmarshal(raw, &result)
	if len(result.Models[0]["supported_reasoning_levels"].([]any)) != 0 || result.Models[0]["default_reasoning_level"] != nil {
		t.Fatal("upstream-only effort exported", result)
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

func TestCatalogProjectsNativeReasoningWithoutMutatingMappings(t *testing.T) {
	for _, strict := range []bool{false, true} {
		for _, tc := range []struct {
			name        string
			values      []*string
			def         *string
			want        []string
			wantDefault any
		}{
			{"mixed", []*string{settingsPtr("medium"), settingsPtr("low"), settingsPtr("xhigh"), settingsPtr("high"), settingsPtr("max"), settingsPtr("default")}, settingsPtr("high"), []string{"medium", "low", "xhigh", "high"}, "high"},
			{"native-default", []*string{settingsPtr("max"), settingsPtr("high"), settingsPtr("default")}, settingsPtr("default"), []string{"high"}, nil},
			{"native-max", []*string{settingsPtr("max")}, settingsPtr("max"), []string{}, nil},
			{"native-null", []*string{nil, settingsPtr("low")}, nil, []string{"low"}, nil},
			{"all-codex-values", []*string{settingsPtr("none"), settingsPtr("minimal"), settingsPtr("low"), settingsPtr("medium"), settingsPtr("high"), settingsPtr("xhigh")}, settingsPtr("none"), []string{"none", "minimal", "low", "medium", "high", "xhigh"}, "none"},
		} {
			t.Run(fmt.Sprintf("strict=%v/%s", strict, tc.name), func(t *testing.T) {
				model := state.Model{ID: "public(fast)", Settings: &state.ModelSettings{Configuration: state.ModelConfiguration{
					SupportedReasoning: []modeldev.ReasoningOption{{Type: "effort", Values: tc.values}},
					DefaultReasoning:   &state.ReasoningDefault{Type: "effort", Value: tc.def},
				}}}
				version := ""
				if strict {
					model.Codex = metadata(t)
					version = model.Codex.Version
				}
				before, _ := json.Marshal(model)
				raw, _, err := buildCatalog([]state.Model{model}, version)
				if err != nil {
					t.Fatal(err)
				}
				var catalog struct {
					Models []struct {
						Levels []struct {
							Effort string `json:"effort"`
						} `json:"supported_reasoning_levels"`
						Default any `json:"default_reasoning_level"`
					} `json:"models"`
				}
				if err = json.Unmarshal(raw, &catalog); err != nil {
					t.Fatal(err)
				}
				got := []string{}
				for _, level := range catalog.Models[0].Levels {
					got = append(got, level.Effort)
				}
				if !reflect.DeepEqual(got, tc.want) || catalog.Models[0].Default != tc.wantDefault {
					t.Fatalf("efforts=%v default=%v", got, catalog.Models[0].Default)
				}
				after, _ := json.Marshal(model)
				if !bytes.Equal(before, after) {
					t.Fatal("stored model settings/metadata mutated")
				}
			})
		}
	}
}
