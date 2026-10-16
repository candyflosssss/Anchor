package protocol

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSafeServiceTier(t *testing.T) {
	for _, tier := range []string{"auto", "default", "priority", "fast", "ultrafast", "flex", "scale"} {
		if got := SafeServiceTier(tier); got != tier {
			t.Fatalf("%s: %s", tier, got)
		}
	}
	for _, tier := range []any{nil, "", "SECRET", 1, Object{"secret": "SECRET"}, []any{"SECRET"}} {
		if got := SafeServiceTier(tier); got != "unknown" {
			t.Fatalf("unsafe tier retained: %s", got)
		}
	}
	if ReadServiceTier(Object{}, "responses") != "" || ReadServiceTier(Object{"service_tier": "priority"}, "anthropic") != "" {
		t.Fatal("missing or unrelated metadata must remain unknown")
	}
}

func TestServiceTierStreamObservation(t *testing.T) {
	for _, to := range []string{"responses", "chat-completions"} {
		for _, tier := range []string{"priority", "ultrafast", "default", "SECRET", ""} {
			t.Run(to+"/"+tier, func(t *testing.T) {
				field := ""
				want := ""
				if tier != "" {
					field = `,"service_tier":"` + tier + `"`
					want = SafeServiceTier(tier)
				}
				sse := `data: {"type":"response.completed","response":{"id":"r","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0}` + field + "}}\n\n"
				o := Observation{}
				if _, err := StreamObserved(httptest.NewRecorder(), strings.NewReader(sse), "responses", to, "m", &o); err != nil || o.ServiceTier != want {
					t.Fatalf("observation=%+v err=%v", o, err)
				}
			})
		}
	}
	for _, failed := range []bool{false, true} {
		end := "completed"
		if failed {
			end = "failed"
		}
		sse := "data: {\"type\":\"response.created\",\"response\":{\"service_tier\":\"priority\"}}\n\ndata: {\"type\":\"response." + end + "\",\"response\":{\"status\":\"" + end + "\",\"service_tier\":\"default\"}}\n\n"
		o := Observation{}
		_, err := StreamObserved(httptest.NewRecorder(), strings.NewReader(sse), "responses", "responses", "m", &o)
		if o.ServiceTier != "default" || (err != nil) != failed {
			t.Fatalf("terminal tier must override initial tier, including failures: %+v %v", o, err)
		}
	}
	o := Observation{}
	_, err := StreamObserved(httptest.NewRecorder(), strings.NewReader("data: {\"service_tier\":\"ultrafast\",\"choices\":[]}\n\ndata: [DONE]\n\n"), "chat-completions", "chat-completions", "m", &o)
	if err != nil || o.ServiceTier != "ultrafast" {
		t.Fatalf("chat tier: %+v %v", o, err)
	}
}

func TestRawServiceTier(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{"priority", `"priority"`}, {"turbo-v2", `"turbo-v2"`}, {"ultrafast", `"ultrafast"`}, {"unknown", `"unknown"`},
		{"", `""`}, {nil, `null`}, {false, `false`}, {float64(42), `42`},
		{Object{"secret": "DO_NOT_STORE"}, ""}, {[]any{"DO_NOT_STORE"}, ""},
		{strings.Repeat("x", 129), ""}, {"hello\nworld", ""}, {"sk-test-only", ""},
		{"Bearer private", ""}, {"private-key", ""},
	} {
		if got := RawServiceTier(tc.value, "private-key"); got != tc.want {
			t.Fatalf("raw tier got %q, want %q", got, tc.want)
		}
	}
	if ReadRawServiceTier([]byte(`{}`), "responses") != "" || ReadRawServiceTier([]byte(`{"service_tier":"fast"}`), "anthropic") != "" {
		t.Fatal("missing/unsupported tier recorded")
	}
}

func TestRawServiceTierStreamReplacesEarlierObservation(t *testing.T) {
	for _, protocol := range []string{"responses", "chat-completions"} {
		for _, value := range []any{"turbo-v2", nil, Object{"secret": "DO_NOT_STORE"}} {
			o := &Observation{}
			first, last := Object{"service_tier": "priority"}, Object{"service_tier": value}
			if protocol == "responses" {
				first = Object{"response": first}
				last = Object{"response": last}
			}
			firstRaw, _ := json.Marshal(first)
			lastRaw, _ := json.Marshal(last)
			o.observeServiceTier(first, protocol, firstRaw)
			o.observeServiceTier(last, protocol, lastRaw)
			if o.ServiceTier != "unknown" || o.ServiceTierRaw != RawServiceTier(value) {
				t.Fatalf("stale observation: %+v", o)
			}
		}
	}
}

func TestRawServiceTierNumberPrecision(t *testing.T) {
	const raw = "9007199254740993"
	if got := ReadRawServiceTier([]byte(`{"service_tier":`+raw+`}`), "responses"); got != raw {
		t.Fatalf("rounded raw tier: %s", got)
	}
	for _, p := range []string{"responses", "chat-completions"} {
		payload := `{"service_tier":` + raw + `,"choices":[]}`
		if p == "responses" {
			payload = `{"type":"response.completed","response":{"service_tier":` + raw + `,"status":"completed","output":[]}}`
		}
		o := Observation{}
		_, _ = StreamObserved(httptest.NewRecorder(), strings.NewReader("data: "+payload+"\n\ndata: [DONE]\n\n"), p, p, "m", &o)
		if o.ServiceTierRaw != raw || o.ServiceTier != "unknown" {
			t.Fatalf("rounded stream tier: %+v", o)
		}
	}
}
