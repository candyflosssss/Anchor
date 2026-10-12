package catalog

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParseCatalog(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		want  []string
		valid bool
	}{
		{`{"data":[{"id":"z","metadata":"discard"},{"id":"a"},{"id":"z"}]}`, []string{"a", "z"}, true},
		{`{"data":[]}`, []string{}, true},
		{`{"data":null}`, nil, false}, {`{}`, nil, false}, {`[]`, nil, false},
		{`{"data":[null]}`, nil, false}, {`{"data":[{"id":2}]}`, nil, false},
		{`{"data":[{"id":"bad\nname"}]}`, nil, false}, {`{"data":[{"id":" "}]}`, nil, false},
		{`{"data":[{"id":"bad\u202ename"}]}`, nil, false},
		{`{"data":[{"id":"fixture-secret"}]}`, nil, false},
		{`{"data":[]} {}`, nil, false},
		{`{"data":[{"id":"` + strings.Repeat("x", 257) + `"}]}`, nil, false},
		{`{"data":[` + strings.TrimSuffix(strings.Repeat(`{"id":"x"},`, 1001), ",") + `]}`, nil, false},
	} {
		got, err := Parse([]byte(tc.raw), "fixture-secret")
		if (err == nil) != tc.valid || (tc.valid && !reflect.DeepEqual(got, tc.want)) {
			t.Fatalf("valid=%v got=%v err=%v", tc.valid, got, err)
		}
	}
	raw, _ := json.Marshal(map[string]any{"data": []any{}, "large": strings.Repeat("x", MaxBytes)})
	if _, err := Parse(raw, ""); err == nil {
		t.Fatal("accepted oversized input")
	}
}

func TestCatalogFormatsAndPagination(t *testing.T) {
	for _, tc := range []struct {
		name, raw, parameter, cursor string
		models                       []string
		valid                        bool
	}{
		{"anthropic-final", `{"data":[{"id":"claude-test"}],"has_more":false,"last_id":"claude-test"}`, "", "", []string{"claude-test"}, true},
		{"anthropic-next", `{"data":[{"id":"claude-test"}],"has_more":true,"last_id":"claude-test"}`, "after_id", "claude-test", []string{"claude-test"}, true},
		{"anthropic-missing-cursor", `{"data":[{"id":"a"}],"has_more":true}`, "", "", nil, false},
		{"anthropic-wrong-last", `{"data":[{"id":"a"}],"has_more":true,"last_id":"b"}`, "", "", nil, false},
		{"anthropic-empty-more", `{"data":[],"has_more":true,"last_id":"b"}`, "", "", nil, false},
		{"gemini", `{"models":[{"name":"models/gemini-test","displayName":"discard"}]}`, "", "", []string{"gemini-test"}, true},
		{"gemini-empty", `{"models":[]}`, "", "", []string{}, true},
		{"gemini-next", `{"models":[{"name":"models/gemini-test"}],"nextPageToken":"token&next=one"}`, "pageToken", "token&next=one", []string{"gemini-test"}, true},
		{"gemini-empty-name", `{"models":[{"name":"models/"}]}`, "", "", nil, false},
		{"gemini-gateway-name", `{"models":[{"name":"gemini-test"}]}`, "", "", []string{"gemini-test"}, true},
		{"gemini-null", `{"models":[null]}`, "", "", nil, false},
		{"gemini-secret", `{"models":[{"name":"models/fixture-secret"}]}`, "", "", nil, false},
		{"secret-cursor", `{"models":[{"name":"models/a"}],"nextPageToken":"fixture-secret"}`, "", "", nil, false},
		{"control-cursor", `{"models":[{"name":"models/a"}],"nextPageToken":"bad\nname"}`, "", "", nil, false},
		{"empty-more", `{"models":[],"nextPageToken":"a"}`, "", "", nil, false},
		{"ambiguous", `{"models":[],"data":[]}`, "", "", nil, false},
		{"mixed-pagination", `{"data":[{"id":"a"}],"nextPageToken":"x"}`, "", "", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := ParsePage([]byte(tc.raw), "fixture-secret")
			if (err == nil) != tc.valid || (tc.valid && (!reflect.DeepEqual(page.Models, tc.models) || page.NextCursor != tc.cursor || page.CursorParameter != tc.parameter)) {
				t.Fatalf("page=%+v err=%v", page, err)
			}
			if tc.cursor != "" {
				if _, err := Parse([]byte(tc.raw), "fixture-secret"); err == nil {
					t.Fatal("single-page parser accepted unfinished directory")
				}
			}
		})
	}
}
