package logbook

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestDimensionStatisticsAndFilters(t *testing.T) {
	s := openTest(t)
	start := time.Now().Add(-10 * time.Minute).UnixMilli()
	values := []struct {
		id, status, project                  string
		input, output, elapsed, cache, first *int64
		offset                               int64
	}{
		{"good", "succeeded", "anchor", statInt(100), statInt(50), statInt(1000), statInt(80), statInt(100), 0},
		{"bad", "failed", "anchor", nil, nil, statInt(2000), nil, nil, 1000},
		{"legacy", "succeeded", "", statInt(300), statInt(150), statInt(3000), statInt(0), statInt(200), 120000},
	}
	for _, v := range values {
		r := row(v.id)
		r.StartedAtMS, r.Status, r.Project = start+v.offset, v.status, v.project
		if v.project != "" {
			r.Agent, r.Binding, r.Session = "codex", "desktop", "thread-1"
		}
		r.Input, r.Output, r.ElapsedMS, r.AttemptCount = v.input, v.output, v.elapsed, 1
		r.Metrics.CacheReadTokens, r.Metrics.TimeToFirstOutputMS = v.cache, v.first
		s.Request(r)
	}
	flushTest(t, s)
	var fixture struct {
		Stats Stats `json:"stats"`
	}
	raw, err := os.ReadFile("../../desktop/contract/log-dimensions.json")
	if err != nil || json.Unmarshal(raw, &fixture) != nil {
		t.Fatal("fixture", err)
	}
	q := StatsQuery{Filter: Query{SinceMS: start, BeforeMS: start + 180000, Limit: 1, Cursor: 1, Until: 1}, IntervalMS: 60000, GroupBy: "project"}
	got, err := s.Statistics(t.Context(), q)
	if err != nil || got.GroupCount != 2 || !reflect.DeepEqual(got.Groups, fixture.Stats.Groups) || !reflect.DeepEqual(got.Summary, fixture.Stats.Summary) {
		t.Fatalf("group contract drift: %+v %v", got, err)
	}
	for _, dimension := range []string{"agent", "binding", "project", "session"} {
		q.GroupBy = dimension
		grouped, err := s.Statistics(t.Context(), q)
		if err != nil || grouped.GroupCount != 2 || grouped.Groups[0].Values.Requests != 2 || grouped.Groups[1].Value != "" {
			t.Fatalf("%s: %+v %v", dimension, grouped, err)
		}
		selected := ""
		switch dimension {
		case "agent":
			selected = "codex"
			q.Filter.Agent = &selected
		case "binding":
			selected = "desktop"
			q.Filter.Binding = &selected
		case "project":
			selected = "anchor"
			q.Filter.Project = &selected
		case "session":
			selected = "thread-1"
			q.Filter.Session = &selected
		}
		stats, err := s.Statistics(t.Context(), q)
		if err != nil || stats.Summary.Requests != 2 {
			t.Fatal(stats, err)
		}
		listQ := q.Filter
		listQ.Cursor, listQ.Until, listQ.Limit = 0, 0, 50
		page, err := s.List(t.Context(), listQ)
		if err != nil || len(page.Requests) != 2 {
			t.Fatal(page, err)
		}
		selected = "" // The same query now targets missing/legacy values, not all rows.
		stats, err = s.Statistics(t.Context(), q)
		if err != nil || stats.Summary.Requests != 1 {
			t.Fatal(stats, err)
		}
		listQ = q.Filter
		listQ.Cursor, listQ.Until, listQ.Limit = 0, 0, 50
		page, err = s.List(t.Context(), listQ)
		if err != nil || len(page.Requests) != 1 || page.Requests[0].ID != "legacy" {
			t.Fatal(page, err)
		}
		q.Filter.Agent, q.Filter.Binding, q.Filter.Project, q.Filter.Session = nil, nil, nil, nil
		suggestions, err := s.Suggest(t.Context(), SuggestionsQuery{Field: dimension, SinceMS: start, BeforeMS: start + 180000})
		if err != nil || len(suggestions.Items) != 1 || suggestions.Items[0].Count != 2 {
			t.Fatal(suggestions, err)
		}
	}
	for _, search := range []string{"code", "desk", "anch", "thread"} {
		page, err := s.List(t.Context(), Query{SinceMS: start, BeforeMS: start + 180000, Search: search})
		if err != nil || len(page.Requests) != 2 {
			t.Fatal(search, page, err)
		}
	}

}

func TestDimensionBounds(t *testing.T) {
	if q := (StatsQuery{GroupBy: "document"}); q.Validate() == nil {
		t.Fatal("untrusted grouping accepted")
	}
	empty := ""
	if q := (Query{Kind: "system", Project: &empty}); q.Validate() == nil {
		t.Fatal("system attribution filter silently ignored")
	}
	s := openTest(t)
	start := time.Now().Add(-time.Minute).UnixMilli()
	for i := 0; i < 125; i++ {
		r := row(fmt.Sprintf("group-%03d", i))
		r.StartedAtMS = start
		r.Session = r.ID
		s.Request(r)
	}
	flushTest(t, s)
	stats, err := s.Statistics(t.Context(), StatsQuery{Filter: Query{SinceMS: start, BeforeMS: start + 10000}, GroupBy: "session"})
	if err != nil || stats.GroupCount != 125 || len(stats.Groups) != 100 || stats.Summary.Requests != 125 || stats.Groups[0].Value != "group-000" {
		t.Fatal(stats, err)
	}
}

func TestDimensionIndicesReopenLegacyDocuments(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Request(row("legacy"))
	flushTest(t, s)
	for _, name := range []string{"request_agent", "request_binding", "request_project", "request_session"} {
		if _, err = s.write.Exec("DROP INDEX " + name); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var count int
	if err = s.read.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='index' AND name IN ('request_agent','request_binding','request_project','request_session')").Scan(&count); err != nil || count != 4 {
		t.Fatal(count, err)
	}
	empty := ""
	page, err := s.List(t.Context(), Query{Agent: &empty, Project: &empty})
	if err != nil || len(page.Requests) != 1 || page.Requests[0].Agent != "" {
		t.Fatal(page, err)
	}
	var populated int
	if err = s.read.QueryRow("SELECT count(*) FROM requests WHERE json_type(document,'$.agent') IS NOT NULL").Scan(&populated); err != nil || populated != 0 {
		t.Fatal("legacy data rewritten", populated, err)
	}
}
