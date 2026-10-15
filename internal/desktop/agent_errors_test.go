package desktop

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/candyflosssss/Anchor/internal/client"
)

func TestAgentProblemPreservesOnlyAllowlistedFailureCodes(t *testing.T) {
	codes := []string{"agent_operation_failed", "agent_model_unavailable", "agent_catalog_invalid", "agent_config_conflict", "agent_config_unsafe"}
	raw, err := os.ReadFile("../../desktop/contract/agents.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		FailureCodes []string `json:"failure_codes"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fixture.FailureCodes, codes) {
		t.Fatal("shared failure contract drift")
	}
	for _, code := range codes {
		for _, writing := range []bool{false, true} {
			got := problem(&client.RequestError{Status: 409, Code: code}, writing)
			if got.Code != code {
				t.Fatalf("%s collapsed to %s", code, got.Code)
			}
		}
		if got := problem(&client.RequestError{Status: 409, Code: code, Unknown: true}, true); got.Code != "unknown" {
			t.Fatal("unknown mutation outcome was hidden", got.Code)
		}
	}
	if got := problem(&client.RequestError{Status: 409, Code: "untrusted_private_text"}, true); got.Code != "conflict" {
		t.Fatal("arbitrary upstream code crossed desktop boundary", got.Code)
	}
}
