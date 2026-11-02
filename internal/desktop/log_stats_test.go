package desktop

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSharedLogStatsContract(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/log-stats.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Contract string        `json:"contract"`
		Query    LogStatsQuery `json:"query"`
		Stats    LogStats      `json:"stats"`
	}
	if json.Unmarshal(raw, &f) != nil || f.Contract != Contract || f.Query.Validate() != nil || len(f.Stats.Buckets) != 3 || f.Stats.Summary.UsageSamples != 2 || f.Stats.Buckets[1].Values.TPS != nil || *f.Stats.Summary.CacheHitRate != 20 {
		t.Fatal("statistics fixture drift")
	}
	if f.Stats.Summary.CostUSD == nil || *f.Stats.Summary.CostUSD != "0.000492" || *f.Stats.Summary.CostSamples != 2 || f.Stats.Buckets[1].Values.CostUSD != nil || *f.Stats.Buckets[2].Values.CostUSD != "0" {
		t.Fatal("cost fixture drift")
	}
	b := NewBridge(testSession(t))
	requireReady(t, b)
	v := b.LogStats(LogStatsQuery{})
	if v.Problem != nil || v.Data == nil || v.Data.Summary.Requests != 0 || *v.Data.Summary.CostSamples != 0 || v.Data.Summary.CostUSD != nil || len(v.Data.Buckets) == 0 {
		t.Fatal(v)
	}
	if v := b.LogStats(LogStatsQuery{IntervalMS: 123}); v.Problem == nil || v.Problem.Code != "invalid" {
		t.Fatal(v)
	}
	v = b.LogStats(LogStatsQuery{IncludeErrors: true})
	if v.Problem != nil || v.Data == nil || v.Data.ErrorSummary == nil || v.Data.ErrorSummary.Total != 0 || len(v.Data.ErrorSummary.Groups) != 0 {
		t.Fatal(v)
	}
	if !f.Query.IncludeErrors || f.Stats.ErrorSummary == nil || f.Stats.ErrorSummary.Groups[0].Code != "invalid_response" {
		t.Fatal("error summary fixture drift")
	}
}

func TestSharedLogDimensionsContract(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/log-dimensions.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Contract string        `json:"contract"`
		Query    LogStatsQuery `json:"query"`
		Stats    LogStats      `json:"stats"`
	}
	if err = json.Unmarshal(raw, &f); err != nil || f.Contract != Contract || f.Query.Validate() != nil || f.Stats.GroupBy != "project" || f.Stats.GroupCount != 2 || len(f.Stats.Groups) != 2 || f.Stats.Groups[1].Value != "" {
		t.Fatal("dimensions contract drift", err)
	}
	empty := ""
	raw, err = json.Marshal(LogQuery{Project: &empty})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil || string(fields["project"]) != `""` || fields["agent"] != nil {
		t.Fatal("all and unknown filters collapsed", string(raw), err)
	}
}

// A new host connected to an older core must not turn an absent capability
// into a measured zero when decoding and re-encoding the management response.
func TestLogStatsLegacyCostCapability(t *testing.T) {
	var values LogStatsValues
	if err := json.Unmarshal([]byte(`{"requests":1}`), &values); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, present := fields["cost_samples"]; present {
		t.Fatal("legacy core gained cost capability", string(raw))
	}
}
