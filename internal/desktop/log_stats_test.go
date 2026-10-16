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
}
