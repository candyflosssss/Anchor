package logbook

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestStatisticsFrozenCostsAcrossFullRange(t *testing.T) {
	s := openTest(t)
	start := time.Now().Add(-10 * time.Minute).UnixMilli()
	add := func(id, project, status string, at int64, final *string, currency string) {
		r := row(id)
		r.Project, r.Status, r.StartedAtMS = project, status, at
		raw := "999"
		r.ReferenceCost = &ReferenceCost{USD: &raw, Currency: currency}
		if final != nil {
			r.ReferenceCost.Adjustment = &CostAdjustment{USD: final}
		}
		if !s.Request(r) {
			t.Fatal("enqueue")
		}
	}
	amount, free, excluded := "0.000000000000000000000000000001", "0", "1000"
	for i := 0; i < 65; i++ {
		add(fmt.Sprintf("paid_%d", i), "paid", "succeeded", start, &amount, "USD")
	}
	add("free", "free", "succeeded", start+120000, &free, "USD")
	add("legacy", "paid", "failed", start, nil, "USD")
	add("wrong_currency", "paid", "succeeded", start, &excluded, "CNY")
	add("exclusive_boundary", "paid", "succeeded", start+180000, &excluded, "USD")
	flushTest(t, s)
	q := StatsQuery{GroupBy: "project", Filter: Query{SinceMS: start, BeforeMS: start + 180000, Limit: 1, Cursor: 1, Until: 1}, IntervalMS: 60000}
	got, err := s.Statistics(context.Background(), q)
	want := "0.000000000000000000000000000065"
	if err != nil || got.Summary.CostUSD == nil || *got.Summary.CostUSD != want || *got.Summary.CostSamples != 66 || got.Summary.Requests != 68 {
		t.Fatalf("cost summary: %+v, %v", got.Summary, err)
	}
	if *got.Buckets[0].Values.CostSamples != 65 || *got.Buckets[0].Values.CostUSD != want || got.Buckets[1].Values.CostUSD != nil || *got.Buckets[1].Values.CostSamples != 0 || *got.Buckets[2].Values.CostUSD != "0" {
		t.Fatalf("bucket costs: %+v", got.Buckets)
	}
	if len(got.Groups) != 2 || *got.Groups[0].Values.CostSamples != 65 || *got.Groups[0].Values.CostUSD != want || *got.Groups[1].Values.CostSamples != 1 || *got.Groups[1].Values.CostUSD != "0" {
		t.Fatalf("group costs: %+v", got.Groups)
	}
	q.Filter.Status = "failed"
	unknown, err := s.Statistics(t.Context(), q)
	if err != nil || unknown.Summary.Requests != 1 || unknown.Summary.CostUSD != nil || *unknown.Summary.CostSamples != 0 {
		t.Fatal(unknown, err)
	}
	q.Filter.Status = ""
	q.Filter.Project = new(string)
	*q.Filter.Project = "free"
	zero, err := s.Statistics(t.Context(), q)
	if err != nil || zero.Summary.CostUSD == nil || *zero.Summary.CostUSD != "0" || *zero.Summary.CostSamples != 1 {
		t.Fatal(zero, err)
	}
}

func TestStatisticsCostDecimalValidation(t *testing.T) {
	for _, raw := range []string{"", "-1", "NaN", "Infinity", "1/2", "1e2", " 1", "01", "0." + strings.Repeat("1", 31), strings.Repeat("9", 129)} {
		if statsCost(sql.NullString{String: raw, Valid: true}) != nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	if statsCost(sql.NullString{String: "1"}) != nil {
		t.Fatal("missing decimal became known")
	}
	var a statsAccumulator
	for _, raw := range []string{"0.1", "0.2", "0"} {
		a.addCost(statsCost(sql.NullString{String: raw, Valid: true}))
	}
	v := a.values()
	if v.CostUSD == nil || *v.CostUSD != "0.3" || *v.CostSamples != 3 {
		t.Fatal(v)
	}
}
