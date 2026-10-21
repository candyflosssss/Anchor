package logbook

import (
	"fmt"
	"testing"
	"time"
)

func TestStatisticsErrorGroupsFullRange(t *testing.T) {
	s := openTest(t)
	start := time.Now().Add(-time.Minute).UnixMilli()
	http := 200
	add := func(id, status, code, stage string, at int64, upstream *int) {
		r := row(id)
		r.Status = status
		r.Code = code
		r.Stage = stage
		r.StartedAtMS = at
		r.HTTP = upstream
		if !s.Request(r) {
			t.Fatal("enqueue")
		}
	}
	for i := 0; i < 65; i++ {
		add(fmt.Sprintf("failed_%02d", i), "failed", "invalid_response", "response_validate", start+int64(i), &http)
	}
	add("interrupted", "interrupted", "stream_interrupted", "stream", start+100, nil)
	add("cancelled", "cancelled", "invalid_response", "response_validate", start+100, &http)
	add("pending", "pending", "", "", start+100, nil)
	add("success", "succeeded", "", "", start+100, &http)
	add("before", "failed", "invalid_response", "response_validate", start-1, &http)
	add("end", "failed", "invalid_response", "response_validate", start+10000, &http)
	flushTest(t, s)
	q := StatsQuery{IncludeErrors: true, Filter: Query{SinceMS: start, BeforeMS: start + 10000, Limit: 1, Cursor: 1, Until: 1}}
	v, err := s.Statistics(t.Context(), q)
	if err != nil {
		t.Fatal(err)
	}
	e := v.ErrorSummary
	if e == nil || e.Total != 66 || e.GroupCount != 2 || len(e.Groups) != 2 || v.Summary.Errors != e.Total {
		t.Fatalf("%+v", v)
	}
	if e.Groups[0].Count != 65 || e.Groups[0].RequestID != "failed_64" || e.Groups[0].FirstMS != start || e.Groups[0].LastMS != start+64 || e.Groups[0].HTTP == nil || *e.Groups[0].HTTP != 200 || e.Groups[1].HTTP != nil {
		t.Fatalf("%+v", e)
	}
	q.Filter.Cursor, q.Filter.Until = 0, 0
	q.Filter.ErrorsOnly = true
	page, err := s.List(t.Context(), Query{Kind: "requests", SinceMS: start, BeforeMS: start + 9999, ErrorsOnly: true, Limit: 200})
	if err != nil || len(page.Requests) != 66 {
		t.Fatalf("list errors: %d %v", len(page.Requests), err)
	}
	q.Filter.Status = "failed"
	v, err = s.Statistics(t.Context(), q)
	if err != nil || v.ErrorSummary.Total != 65 || v.Summary.Requests != 65 {
		t.Fatalf("%+v %v", v, err)
	}
	q.Filter.Status = "succeeded"
	v, err = s.Statistics(t.Context(), q)
	if err != nil || v.ErrorSummary.Total != 0 || len(v.ErrorSummary.Groups) != 0 {
		t.Fatalf("%+v %v", v, err)
	}
	q.Filter.Status = ""
	q.IncludeErrors = false
	v, err = s.Statistics(t.Context(), q)
	if err != nil || v.ErrorSummary != nil {
		t.Fatalf("optional summary: %+v %v", v, err)
	}
	invalid := Query{Kind: "system", ErrorsOnly: true}
	if invalid.Validate() == nil {
		t.Fatal("system errors_only accepted")
	}
}

func TestStatisticsErrorGroupsCapAndStableOrder(t *testing.T) {
	s := openTest(t)
	start := time.Now().Add(-time.Minute).UnixMilli()
	for i := 0; i < 105; i++ {
		r := row(fmt.Sprintf("error_%03d", i))
		r.Status = "failed"
		r.Code = fmt.Sprintf("error_%03d", i)
		r.StartedAtMS = start
		if !s.Request(r) {
			t.Fatal("enqueue")
		}
	}
	flushTest(t, s)
	v, err := s.Statistics(t.Context(), StatsQuery{IncludeErrors: true, Filter: Query{SinceMS: start, BeforeMS: start + 10000}})
	if err != nil {
		t.Fatal(err)
	}
	e := v.ErrorSummary
	if e == nil || e.Total != 105 || e.GroupCount != 105 || len(e.Groups) != 100 || e.Groups[0].Code != "error_000" || e.Groups[99].Code != "error_099" {
		t.Fatalf("%+v", e)
	}
}
