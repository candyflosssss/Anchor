package logbook

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestSuggestionsFullHistoryScopeAndOrdering(t *testing.T) {
	s := openTest(t)
	now := time.Now().UnixMilli()
	for i := 0; i < 75; i++ {
		r := row(fmt.Sprintf("request_%03d", i))
		r.StartedAtMS, r.Provider, r.Model = now-1000, "frequent", "shared"
		if i == 0 {
			r.Provider = "older-page"
		}
		s.Request(r)
	}
	for _, name := range []string{"a-tie", "z-tie"} {
		r := row(name)
		r.StartedAtMS, r.Provider, r.Model = now-1000, name, name
		s.Request(r)
	}
	r := row("outside-window")
	r.StartedAtMS, r.Provider = now-int64(2*time.Hour/time.Millisecond), "outside"
	s.Request(r)
	flushTest(t, s)
	q := SuggestionsQuery{Field: "provider", SinceMS: now - 60000, BeforeMS: now, Limit: 3}
	out, err := s.Suggest(t.Context(), q)
	want := []Suggestion{{"frequent", 74}, {"a-tie", 1}, {"older-page", 1}}
	if err != nil || !out.HasMore || !reflect.DeepEqual(out.Items, want) {
		t.Fatalf("full history/ranking: %+v %v", out, err)
	}
	q.Prefix = "older"
	out, err = s.Suggest(t.Context(), q)
	if err != nil || out.HasMore || !reflect.DeepEqual(out.Items, []Suggestion{{"older-page", 1}}) {
		t.Fatalf("prefix on older page: %+v %v", out, err)
	}
	q.Field, q.Prefix = "search", "a-tie"
	out, err = s.Suggest(t.Context(), q)
	if err != nil || !reflect.DeepEqual(out.Items, []Suggestion{{"a-tie", 1}}) {
		t.Fatalf("same value in id/model/provider counted more than once: %+v %v", out, err)
	}
	for _, prefix := range []string{"A-TIE", "%", "' OR 1=1 --", "outside"} {
		q.Prefix = prefix
		out, err = s.Suggest(t.Context(), q)
		if err != nil || out.Items == nil || len(out.Items) != 0 || out.HasMore {
			t.Fatalf("literal/case-sensitive prefix %q: %+v %v", prefix, out, err)
		}
	}
}

func TestSuggestionsFieldsAndSystem(t *testing.T) {
	s := openTest(t)
	r := row("safe-id")
	http := 429
	r.Model, r.Provider, r.Key, r.Code, r.Stage, r.HTTP = "model", "provider", "key-id", "rate_limited", "dispatch", &http
	r.Evidence["raw"] = "NOT_A_CANDIDATE"
	s.Request(r)
	empty := row("empty")
	empty.Model, empty.Provider, empty.Key, empty.Code, empty.Stage = "", "", "", "", ""
	s.Request(empty)
	s.Event(Event{ID: "event-id", AtMS: time.Now().UnixMilli(), Event: "core.ready", Level: "info", Source: "core", Fields: map[string]string{"note": "NOT_A_CANDIDATE"}})
	flushTest(t, s)
	for field, value := range map[string]string{"model": "model", "provider": "provider", "key": "key-id", "code": "rate_limited", "stage": "dispatch", "http": "429"} {
		out, err := s.Suggest(t.Context(), SuggestionsQuery{Field: field})
		if err != nil || !reflect.DeepEqual(out.Items, []Suggestion{{value, 1}}) {
			t.Fatalf("field %s: %+v %v", field, out, err)
		}
	}
	out, err := s.Suggest(t.Context(), SuggestionsQuery{Kind: "system", Field: "search"})
	if err != nil || !reflect.DeepEqual(out.Items, []Suggestion{{"core.ready", 1}}) {
		t.Fatalf("system search: %+v %v", out, err)
	}
	out, err = s.Suggest(t.Context(), SuggestionsQuery{Field: "search", Prefix: "NOT_A"})
	if err != nil || len(out.Items) != 0 {
		t.Fatal("document fields exposed", out, err)
	}
}

func TestSuggestionsValidation(t *testing.T) {
	for _, q := range []SuggestionsQuery{
		{Field: "document"}, {Field: "provider; DROP TABLE requests"}, {Field: "search", Kind: "unknown"},
		{Field: "key", Kind: "system"}, {Field: "search", Limit: 101}, {Field: "search", Limit: -1},
		{Field: "search", SinceMS: -1}, {Field: "search", SinceMS: 10, BeforeMS: 9},
		{Field: "search", SinceMS: 1, BeforeMS: int64(91 * 24 * time.Hour / time.Millisecond)},
		{Field: "search", Prefix: string(make([]byte, 513))},
	} {
		if q.Validate() == nil {
			t.Fatalf("accepted invalid query: %+v", q)
		}
	}
}
