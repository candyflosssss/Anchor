package managed

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func dshSettingsFixture() state.ModelConfiguration {
	return state.ModelConfiguration{
		Limits:              modeldev.CatalogLimits{Context: settingsPtr(int64(200000)), Output: settingsPtr(int64(16000))},
		Capabilities:        modeldev.CatalogCapabilities{Vision: settingsPtr(true), Reasoning: settingsPtr(true)},
		SupportedReasoning:  []modeldev.ReasoningOption{{Type: "effort", Values: []*string{settingsPtr("off"), settingsPtr("low"), settingsPtr("high"), settingsPtr("max"), settingsPtr("default"), nil}}},
		DefaultReasoning:    &state.ReasoningDefault{Type: "effort", Value: settingsPtr("high")},
		DefaultOutputTokens: settingsPtr(int64(8000)),
	}
}

func TestDSHModelProjection(t *testing.T) {
	model := state.Model{ID: "public(fast)", DisplayName: "Readable name", Settings: &state.ModelSettings{Configuration: dshSettingsFixture()}}
	before, _ := json.Marshal(model)
	entry, def, err := dshModel(model)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"id": "public(fast)", "name": "Readable name", "contextWindow": int64(200000), "maxTokens": int64(8000), "input": []string{"text", "image"}, "reasoningEfforts": map[string]string{"off": "off", "low": "low", "high": "high", "max": "max"}}
	if !reflect.DeepEqual(entry, want) || def != "high" {
		t.Fatalf("entry=%#v default=%v", entry, def)
	}
	after, _ := json.Marshal(model)
	if !bytes.Equal(before, after) {
		t.Fatal("projection mutated mapping settings")
	}
}

func TestDSHModelUnknownAndNarrowCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings *state.ModelSettings
		want     map[string]any
	}{
		{"unknown", nil, map[string]any{}},
		{"unknown-settings", &state.ModelSettings{}, map[string]any{}},
		{"explicit-no-vision", &state.ModelSettings{Configuration: state.ModelConfiguration{Capabilities: modeldev.CatalogCapabilities{Vision: settingsPtr(false)}}}, map[string]any{"input": []string{"text"}}},
		{"modalities", &state.ModelSettings{Configuration: state.ModelConfiguration{Capabilities: modeldev.CatalogCapabilities{InputModalities: []string{"text", "image", "audio"}}}}, map[string]any{"input": []string{"text", "image"}}},
		{"unsupported-modality", &state.ModelSettings{Configuration: state.ModelConfiguration{Capabilities: modeldev.CatalogCapabilities{InputModalities: []string{"audio"}}}}, map[string]any{}},
		{"output-capacity-not-default", &state.ModelSettings{Configuration: state.ModelConfiguration{Limits: modeldev.CatalogLimits{Output: settingsPtr(int64(16000))}}}, map[string]any{}},
		{"reasoning-disabled", &state.ModelSettings{Configuration: state.ModelConfiguration{Capabilities: modeldev.CatalogCapabilities{Reasoning: settingsPtr(false)}}}, map[string]any{"reasoningEfforts": false}},
		{"reasoning-flag-is-not-levels", &state.ModelSettings{Configuration: state.ModelConfiguration{Capabilities: modeldev.CatalogCapabilities{Reasoning: settingsPtr(true)}}}, map[string]any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.want["id"], tc.want["name"] = "alias", "alias"
			entry, def, err := dshModel(state.Model{ID: "alias", Settings: tc.settings})
			if err != nil || def != nil || !reflect.DeepEqual(entry, tc.want) {
				t.Fatal(entry, def, err)
			}
		})
	}
}

func TestDSHReasoningPreservesNativeSemantics(t *testing.T) {
	for _, tc := range []struct {
		name        string
		options     []modeldev.ReasoningOption
		def         *state.ReasoningDefault
		want        any
		wantDefault any
	}{
		{"levels", []modeldev.ReasoningOption{{Type: "effort", Values: []*string{settingsPtr("minimal"), settingsPtr("low"), settingsPtr("medium"), settingsPtr("high"), settingsPtr("xhigh"), settingsPtr("max")}}}, &state.ReasoningDefault{Type: "effort", Value: settingsPtr("max")}, map[string]string{"minimal": "minimal", "low": "low", "medium": "medium", "high": "high", "xhigh": "xhigh", "max": "max"}, "max"},
		{"native-null", []modeldev.ReasoningOption{{Type: "effort", Values: []*string{nil, settingsPtr("low")}}}, &state.ReasoningDefault{Type: "effort"}, map[string]string{"low": "low"}, nil},
		{"native-default", []modeldev.ReasoningOption{{Type: "effort", Values: []*string{settingsPtr("default"), settingsPtr("high")}}}, &state.ReasoningDefault{Type: "effort", Value: settingsPtr("default")}, map[string]string{"high": "high"}, nil},
		{"unrepresentable", []modeldev.ReasoningOption{{Type: "effort", Values: []*string{nil, settingsPtr("none"), settingsPtr("default")}}}, nil, false, nil},
		{"off-only-invalid-in-dsh", []modeldev.ReasoningOption{{Type: "effort", Values: []*string{settingsPtr("off")}}}, nil, false, nil},
		{"toggle", []modeldev.ReasoningOption{{Type: "toggle"}}, &state.ReasoningDefault{Type: "toggle", Enabled: settingsPtr(true)}, false, nil},
		{"budget", []modeldev.ReasoningOption{{Type: "budget_tokens", Min: settingsPtr(int64(0)), Max: settingsPtr(int64(8000))}}, &state.ReasoningDefault{Type: "budget_tokens", BudgetTokens: settingsPtr(int64(4000))}, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry, def, err := dshModel(state.Model{ID: "alias", Settings: &state.ModelSettings{Configuration: state.ModelConfiguration{SupportedReasoning: tc.options, DefaultReasoning: tc.def}}})
			if err != nil || !reflect.DeepEqual(entry["reasoningEfforts"], tc.want) || def != tc.wantDefault {
				t.Fatal(entry, def, err)
			}
		})
	}
}

func TestDSHRejectsInvalidSettings(t *testing.T) {
	for _, cfg := range []state.ModelConfiguration{
		{Limits: modeldev.CatalogLimits{Context: settingsPtr(int64(-1))}},
		{Capabilities: modeldev.CatalogCapabilities{Vision: settingsPtr(false), InputModalities: []string{"image"}}},
		{SupportedReasoning: []modeldev.ReasoningOption{{Type: "effort", Values: []*string{settingsPtr("low")}}}, DefaultReasoning: &state.ReasoningDefault{Type: "effort", Value: settingsPtr("high")}},
	} {
		if _, _, err := dshModel(state.Model{ID: "bad", Settings: &state.ModelSettings{Configuration: cfg}}); err == nil {
			t.Fatal("accepted invalid settings")
		}
	}
}
