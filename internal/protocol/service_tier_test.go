package protocol

import (
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
