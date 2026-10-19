package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestSharedProviderContractFixtures(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/providers.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Contract  string             `json:"contract"`
		Providers []Provider         `json:"providers"`
		Empty     []Provider         `json:"empty"`
		Errors    map[string]Problem `json:"errors"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Contract != Contract || len(fixture.Providers) != 3 || fixture.Empty == nil || len(fixture.Empty) != 0 {
		t.Fatal("fixture contract drift")
	}
	var stateRows struct {
		Providers []state.Provider `json:"providers"`
	}
	if err := json.Unmarshal(raw, &stateRows); err != nil {
		t.Fatal(err)
	}
	for i, p := range stateRows.Providers {
		if err := p.Validate(); err != nil {
			t.Fatal(err)
		}
		dto, ok := provider(p)
		if !ok || !reflect.DeepEqual(dto, fixture.Providers[i]) {
			t.Fatal("public DTO drift")
		}
		encoded, _ := json.Marshal(dto)
		var fields map[string]any
		_ = json.Unmarshal(encoded, &fields)
		if len(fields) != 9 {
			t.Fatal("unexpected provider fields")
		}
		if _, ok := fields["secret"]; ok {
			t.Fatal("secret field leaked")
		}
	}
	failures := map[string]error{
		"offline":      &client.RequestError{Code: "offline"},
		"unauthorized": &client.RequestError{Status: 401},
		"read_error":   errors.New("do not reflect this text"),
		"conflict":     &client.RequestError{Status: 409},
		"unknown":      &client.RequestError{Unknown: true},
		"invalid":      &client.RequestError{Status: 400},
	}
	for name, err := range failures {
		if *problem(err, false) != fixture.Errors[name] {
			t.Fatalf("error fixture drift: %s", name)
		}
	}
}

func TestProviderDTORejectsUnsafeValues(t *testing.T) {
	for _, revision := range []int64{-1, 0, MaxSafeRevision + 1} {
		if _, ok := provider(state.Provider{Revision: revision, Protocol: "responses"}); ok {
			t.Fatal("unsafe revision accepted")
		}
	}
	if _, ok := provider(state.Provider{Revision: 1, Protocol: "unsupported"}); ok {
		t.Fatal("protocol accepted")
	}
}

func TestCoreSourceRequiresSpawnedProcessIdentity(t *testing.T) {
	for _, tc := range []struct {
		launched, responding int
		source               string
	}{
		{0, 0, "reused"}, {0, 52, "reused"}, {41, 0, "reused"},
		{41, 52, "reused"}, // concurrent CLI won the instance lock
		{41, 41, "started"},
	} {
		s := &Session{launchedPID: tc.launched}
		if got := s.source(client.Instance{ProcessID: tc.responding}); got != tc.source {
			t.Fatalf("launch=%d responding=%d: %s, want %s", tc.launched, tc.responding, got, tc.source)
		}
	}
}

func TestSharedProviderModelsContract(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/provider-models.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Contract      string                 `json:"contract"`
		Input         DiscoverProviderModels `json:"input"`
		Success       ProviderModels         `json:"success"`
		Empty         ProviderModels         `json:"empty"`
		Unsupported   ProviderModels         `json:"unsupported"`
		Aggregate     ProviderModels         `json:"aggregate"`
		Partial       ProviderModels         `json:"partial"`
		NoEnabledKeys ProviderModels         `json:"no_enabled_keys"`
	}
	if json.Unmarshal(raw, &fixture) != nil || fixture.Contract != Contract {
		t.Fatal("catalog fixture drift")
	}
	for _, result := range []ProviderModels{fixture.Success, fixture.Empty, fixture.Unsupported, fixture.Aggregate, fixture.Partial, fixture.NoEnabledKeys} {
		if result.ProviderID != fixture.Input.ProviderID || result.Revision != fixture.Input.ExpectedRevision || result.Models == nil || result.Entries == nil || result.Keys == nil {
			t.Fatal("catalog identity drift")
		}
		encoded, _ := json.Marshal(result)
		var fields map[string]any
		_ = json.Unmarshal(encoded, &fields)
		if len(fields) != 10 {
			t.Fatal("unexpected catalog public fields")
		}
	}
	if fixture.NoEnabledKeys.Failure != "no_enabled_keys" || len(fixture.NoEnabledKeys.Keys) != 0 || fixture.NoEnabledKeys.HTTPStatus != 0 || !fixture.Aggregate.Keys[1].Enabled {
		t.Fatal("enabled key catalog contract drift")
	}
	if fixture.Aggregate.KeyID != "" || len(fixture.Aggregate.Entries) != 2 || len(fixture.Aggregate.Entries[0].KeyIDs) != 2 || len(fixture.Aggregate.Keys) != 2 || fixture.Aggregate.Keys[1].Multiplier == nil || *fixture.Aggregate.Keys[1].Multiplier != "1.5" || !fixture.Partial.Partial || fixture.Partial.Failure != "" || fixture.Partial.Keys[1].Failure != "authentication" {
		t.Fatal("catalog merge/key rate semantics drift")
	}
	if fixture.Success.HTTPStatus != 200 || fixture.Empty.Failure != "" || len(fixture.Empty.Models) != 0 || fixture.Unsupported.Failure != "unsupported" {
		t.Fatal("catalog semantics drift")
	}
}

func TestSharedLogContract(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/logs.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Contract string      `json:"contract"`
		Query    LogQuery    `json:"query"`
		Page     LogPage     `json:"page"`
		Settings LogSettings `json:"settings"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Contract != Contract || fixture.Query.Validate() != nil || fixture.Settings.Validate() != nil || len(fixture.Page.Requests) != 2 {
		t.Fatal("log contract drift")
	}
	failed, ok := fixture.Page.Requests[0], fixture.Page.Requests[1]
	if failed.HTTP == nil || *failed.HTTP != 200 || failed.Code != "invalid_response" || failed.Input != nil || failed.Metrics.TimeToFirstOutputMS != nil || failed.Metrics.TimeToFirstTextMS != nil || ok.Metrics.TimeToFirstOutputMS == nil || *ok.Metrics.TimeToFirstOutputMS != 584 || ok.Metrics.CacheReadTokens == nil || *ok.Metrics.CacheReadTokens != 0 {
		t.Fatal("log null/zero semantics drift")
	}
	if failed.Metrics.OutputGenerationMS != nil || failed.Metrics.NonReasoningOutputTokens != nil || failed.Metrics.OutputIntervalComplete ||
		ok.Metrics.OutputGenerationMS == nil || *ok.Metrics.OutputGenerationMS != 1500 || ok.Metrics.NonReasoningOutputTokens == nil || *ok.Metrics.NonReasoningOutputTokens != 100 || !ok.Metrics.OutputIntervalComplete || ok.Metrics.OutputTextTokens != nil || ok.Metrics.TextIntervalComplete {
		t.Fatal("output TPS contract drift")
	}
	if failed.Evidence["sent_service_tier"] != "unspecified" || failed.Evidence["upstream_service_tier_raw"] != `"priority"` || ok.Evidence["sent_service_tier_raw"] != `"ultrafast"` || ok.Evidence["upstream_service_tier_raw"] != `"ultrafast"` || ok.Evidence["fast_triggered"] != "true" {
		t.Fatal("raw tier/acceleration contract drift")
	}

	if failed.ReferenceCost != nil || ok.ReferenceCost == nil || ok.ReferenceCost.USD == nil || *ok.ReferenceCost.USD != "0.00123" || len(ok.Attempts) != 1 || ok.Attempts[0].ReferenceUSD == nil || *ok.Attempts[0].ReferenceUSD != "0.00123" {
		t.Fatal("reference cost null/decimal contract drift")
	}
	encoded, _ := json.Marshal(failed)
	var keys map[string]any
	json.Unmarshal(encoded, &keys)
	if len(keys) != 26 {
		t.Fatalf("unexpected log keys: %d", len(keys))
	}
}
