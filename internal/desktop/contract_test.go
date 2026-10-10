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
		if len(fields) != 8 {
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
		Contract    string                 `json:"contract"`
		Input       DiscoverProviderModels `json:"input"`
		Success     ProviderModels         `json:"success"`
		Empty       ProviderModels         `json:"empty"`
		Unsupported ProviderModels         `json:"unsupported"`
	}
	if json.Unmarshal(raw, &fixture) != nil || fixture.Contract != Contract {
		t.Fatal("catalog fixture drift")
	}
	for _, result := range []ProviderModels{fixture.Success, fixture.Empty, fixture.Unsupported} {
		if result.ProviderID != fixture.Input.ProviderID || result.Revision != fixture.Input.ExpectedRevision || result.Models == nil {
			t.Fatal("catalog identity drift")
		}
		encoded, _ := json.Marshal(result)
		var fields map[string]any
		_ = json.Unmarshal(encoded, &fields)
		if len(fields) != 6 {
			t.Fatal("unexpected catalog public fields")
		}
	}
	if fixture.Success.HTTPStatus != 200 || fixture.Empty.Failure != "" || len(fixture.Empty.Models) != 0 || fixture.Unsupported.Failure != "unsupported" {
		t.Fatal("catalog semantics drift")
	}
}
