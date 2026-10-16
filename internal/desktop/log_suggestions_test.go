package desktop

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSharedLogSuggestionsContract(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/log-suggestions.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Contract    string              `json:"contract"`
		Query       LogSuggestionsQuery `json:"query"`
		Suggestions LogSuggestions      `json:"suggestions"`
	}
	if json.Unmarshal(raw, &f) != nil || f.Contract != Contract || f.Query.Validate() != nil || len(f.Suggestions.Items) != 2 || f.Suggestions.Items[0].Count != 12 || f.Suggestions.HasMore {
		t.Fatal("suggestions fixture drift")
	}
	b := NewBridge(testSession(t))
	requireReady(t, b)
	v := b.LogSuggestions(LogSuggestionsQuery{Field: "provider"})
	if v.Problem != nil || v.Data == nil || v.Data.Items == nil || len(v.Data.Items) != 0 || v.Data.HasMore {
		t.Fatal(v)
	}
	if v := b.LogSuggestions(LogSuggestionsQuery{Field: "document"}); v.Problem == nil || v.Problem.Code != "invalid" {
		t.Fatal(v)
	}
}
