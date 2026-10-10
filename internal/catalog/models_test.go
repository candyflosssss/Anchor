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
