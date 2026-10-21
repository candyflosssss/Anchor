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
	b := NewBridge(testSession(t))
	requireReady(t, b)
	v := b.LogStats(LogStatsQuery{})
	if v.Problem != nil || v.Data == nil || v.Data.Summary.Requests != 0 || len(v.Data.Buckets) == 0 {
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
