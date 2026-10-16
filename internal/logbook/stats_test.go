package logbook

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"testing"
	"time"
)

func statInt(n int64) *int64 { return &n }
func assertStat(t *testing.T, got *float64, want float64) {
	t.Helper()
	if got == nil || math.Abs(*got-want) > 0.00001 {
		t.Fatalf("got %v, want %v", got, want)
	}
}
func TestStatisticsFullRangeFiltersAndNulls(t *testing.T) {
	s := openTest(t)
	start := time.Now().Add(-10 * time.Minute).UnixMilli()
	add := func(id, status string, at int64, input, output, elapsed, cache, first *int64) {
		r := row(id)
		r.Status = status
		r.AttemptCount = 1
		r.StartedAtMS = at
		r.Input = input
		r.Output = output
		r.ElapsedMS = elapsed
		r.Metrics.CacheReadTokens = cache
		r.Metrics.TimeToFirstOutputMS = first
		if id == "good2" {
			r.Metrics.TimeToFirstOutputMS = nil
			r.Metrics.TimeToFirstTextMS = first
		}
		if !s.Request(r) {
			t.Fatal("enqueue")
		}
	}
	add("good", "succeeded", start, statInt(100), statInt(50), statInt(1000), statInt(80), statInt(100))
	add("bad", "failed", start+1000, nil, nil, statInt(2000), nil, nil)
	add("good2", "succeeded", start+120000, statInt(300), statInt(150), statInt(3000), statInt(0), statInt(200))
	// Exact exclusive bound must not leak into a bucket.
	add("boundary", "failed", start+180000, nil, nil, nil, nil, nil)
	flushTest(t, s)
	q := StatsQuery{Filter: Query{SinceMS: start, BeforeMS: start + 180000, Limit: 1, Cursor: 1, Until: 1}, IntervalMS: 60000}
	v, e := s.Statistics(context.Background(), q)
	if e != nil || len(v.Buckets) != 3 || v.Summary.Requests != 3 || v.Summary.UsageSamples != 2 || *v.Summary.TotalTokens != 600 {
		t.Fatalf("%+v %v", v, e)
	}
	assertStat(t, v.Summary.TPS, 50)
	assertStat(t, v.Summary.ErrorRate, 100.0/3)
	assertStat(t, v.Summary.CacheHitRate, 20)
	assertStat(t, v.Summary.LatencyMS, 2000)
	assertStat(t, v.Summary.FirstOutputMS, 150)
	// Browser mocks and the real store must agree on the same canonical sample.
	raw, err := os.ReadFile("../../desktop/contract/log-stats.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Stats Stats `json:"stats"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v.Summary, fixture.Stats.Summary) || len(v.Buckets) != len(fixture.Stats.Buckets) {
		t.Fatalf("shared statistics summary drift: %+v", v.Summary)
	}
	for i, bucket := range v.Buckets {
		if !reflect.DeepEqual(bucket.Values, fixture.Stats.Buckets[i].Values) {
			t.Fatalf("shared statistics bucket %d drift: %+v", i, bucket.Values)
		}
	}
	if v.Buckets[1].Values.Requests != 0 || v.Buckets[1].Values.TotalTokens != nil || v.Buckets[1].Values.TPS != nil || v.Buckets[1].Values.ErrorRate != nil {
		t.Fatal("empty observations became zero")
	}
	q.Filter.Provider = "missing"
	empty, e := s.Statistics(t.Context(), q)
	if e != nil || empty.Summary.Requests != 0 || empty.Summary.TotalTokens != nil {
		t.Fatal(empty, e)
	}
	q.Filter.Provider = "local"
	q.Filter.Status = "failed"
	q.Filter.HTTP = nil
	q.Filter.Search = "bad"
	failed, e := s.Statistics(t.Context(), q)
	if e != nil || failed.Summary.Requests != 1 || failed.Summary.TotalTokens != nil || failed.Summary.TPS != nil {
		t.Fatal(failed, e)
	}
	assertStat(t, failed.Summary.ErrorRate, 100)
	// More than the UI page size is included; no implicit LIMIT or snapshot cursor.
	for i := 0; i < 65; i++ {
		add(fmt.Sprintf("extra_%d", i), "pending", start+10, nil, nil, nil, nil, nil)
	}
	flushTest(t, s)
	q.Filter.Status = ""
	q.Filter.Search = ""
	v, e = s.Statistics(t.Context(), q)
	if e != nil || v.Summary.Requests != 68 {
		t.Fatal(v.Summary, e)
	}
}
func TestStatisticsUnknownZeroAndCancellation(t *testing.T) {
	var a statsAccumulator
	// Failed output does not influence TPS; cancellation is excluded from error denominator.
	a.add("cancelled", nullable(10), nullable(0), nullable(100), nullable(0), nullable(0), 1)
	a.add("failed", nullable(10), nullable(999), nullable(1), nullable(11), nullInt(), 1)
	a.add("interrupted", nullInt(), nullInt(), nullInt(), nullInt(), nullInt(), 1)
	a.add("pending", nullInt(), nullInt(), nullInt(), nullInt(), nullInt(), 1)
	v := a.values()
	if v.TPS != nil || v.CacheSamples != 1 || v.Completed != 2 {
		t.Fatal(v)
	}
	assertStat(t, v.CacheHitRate, 0)
	assertStat(t, v.ErrorRate, 100)
	a.add("succeeded", nullable(0), nullable(0), nullable(1000), nullable(0), nullInt(), 1)
	assertStat(t, a.values().TPS, 0)
}

func nullable(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }
func nullInt() sql.NullInt64         { return sql.NullInt64{} }

func TestStatisticsValidation(t *testing.T) {
	now := time.Now().UnixMilli()
	for _, q := range []StatsQuery{
		{Filter: Query{Kind: "system"}},
		{Filter: Query{SinceMS: now, BeforeMS: now}},
		{Filter: Query{SinceMS: now - 91*86400000, BeforeMS: now}},
		{IntervalMS: -1}, {IntervalMS: 12345},
		{Filter: Query{SinceMS: now - 86400000, BeforeMS: now}, IntervalMS: 10000},
	} {
		if q.Validate() == nil {
			t.Fatalf("accepted %+v", q)
		}
	}
	q := StatsQuery{Filter: Query{SinceMS: now - 90*86400000, BeforeMS: now}}
	if q.Validate() != nil || q.IntervalMS != 86400000 {
		t.Fatal(q)
	}
	s := openTest(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Statistics(ctx, StatsQuery{}); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestStatisticsWeightedRatios(t *testing.T) {
	var a statsAccumulator
	a.add("succeeded", nullable(100), nullable(100), nullable(1000), nullable(50), nullInt(), 1)
	a.add("succeeded", nullable(900), nullable(100), nullable(3000), nullable(10), nullInt(), 1)
	v := a.values()
	assertStat(t, v.TPS, 50)
	assertStat(t, v.CacheHitRate, 6)
}

func TestStatisticsCacheRequiresSingleAttempt(t *testing.T) {
	var a statsAccumulator
	a.add("succeeded", nullable(200), nullable(100), nullable(3000), nullable(100), nullInt(), 2)
	if a.values().CacheHitRate != nil || a.CacheSamples != 0 {
		t.Fatal("last-attempt cache divided by root input")
	}
}
