package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestServiceTierLogs(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tc := range []struct{ name, request, actual, wantRequest, wantActual string }{
			{"fast", `,"service_tier":"priority"`, `,"service_tier":"priority"`, "priority", "priority"},
			{"fast_alias", `,"service_tier":"fast"`, `,"service_tier":"fast"`, "fast", "fast"},
			{"ultrafast", `,"service_tier":"ultrafast"`, `,"service_tier":"ultrafast"`, "ultrafast", "ultrafast"},
			{"downgrade", `,"service_tier":"priority"`, `,"service_tier":"default"`, "priority", "default"},
			{"unreported", `,"service_tier":"priority"`, "", "priority", ""},
			{"implicit_fast", "", `,"service_tier":"priority"`, "unspecified", "priority"},
			{"custom", `,"service_tier":"turbo-v2"`, `,"service_tier":"turbo-v2"`, "unknown", "unknown"},
			{"null", `,"service_tier":null`, `,"service_tier":null`, "unknown", "unknown"},
			{"scalar", `,"service_tier":42`, `,"service_tier":false`, "unknown", "unknown"},
			{"upstream_credential", `,"service_tier":"test"`, `,"service_tier":"test"`, "unknown", "unknown"},
			{"sensitive", `,"service_tier":"sk-test-only"`, `,"service_tier":"sk-test-only"`, "unknown", "unknown"},
			{"auto", `,"service_tier":"auto"`, "", "auto", ""},
			{"unknown", `,"service_tier":{"secret":"DO_NOT_STORE"}`, `,"service_tier":"custom-tier"`, "unknown", "unknown"},
		} {
			t.Run(tc.name+map[bool]string{false: "/json", true: "/sse"}[stream], func(t *testing.T) {
				f := setup(t)
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var incoming map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
						t.Error(err)
					}
					want := requestedServiceTier(incoming)
					if want != tc.wantRequest {
						t.Errorf("forwarded request tier %s, want %s", want, tc.wantRequest)
					}
					body := `{"id":"r","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0}` + tc.actual + `}`
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":"+body+"}\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						io.WriteString(w, body)
					}
				}))
				defer up.Close()
				f.add(t, "p", up.URL, "test")
				f.model(t, state.Candidate{ProviderID: "p", Model: "upstream"})
				body := `{"model":"public","input":"DO_NOT_STORE","stream":` + map[bool]string{false: "false", true: "true"}[stream] + tc.request + `}`
				resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, body)
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 {
					t.Fatal(resp.StatusCode)
				}
				if err := f.core.logs.Flush(t.Context()); err != nil {
					t.Fatal(err)
				}
				c := logClient(t, f)
				row, err := c.LogDetail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
				if err != nil || row.Status != "succeeded" || row.Evidence["requested_service_tier"] != tc.wantRequest || row.Evidence["upstream_service_tier"] != tc.wantActual {
					t.Fatalf("row=%+v err=%v", row, err)
				}
				if row.Evidence["sent_service_tier"] != tc.wantRequest || row.Evidence["fast_triggered"] != fastTriggered(tc.wantActual) {
					t.Fatalf("wrong sent/confirmed tier: %+v", row.Evidence)
				}
				for _, side := range []struct{ field, fragment string }{{"requested", tc.request}, {"sent", tc.request}, {"upstream", tc.actual}} {
					var fields map[string]json.RawMessage
					if err := json.Unmarshal([]byte("{\"dummy\":0"+side.fragment+"}"), &fields); err != nil {
						t.Fatal(err)
					}
					expected := map[string]string{}
					recordRawServiceTier(expected, "raw", fields["service_tier"], "test")
					if row.Evidence[side.field+"_service_tier_raw"] != expected["raw"] {
						t.Fatalf("raw %s: %+v", side.field, row.Evidence)
					}
				}
				page, err := c.Logs(t.Context(), logbook.Query{})
				if err != nil || len(page.Requests) != 1 {
					t.Fatalf("page=%+v err=%v", page, err)
				}
				for _, name := range []string{"sent_service_tier", "sent_service_tier_raw", "upstream_service_tier", "upstream_service_tier_raw", "fast_triggered"} {
					if page.Requests[0].Evidence[name] != row.Evidence[name] {
						t.Fatalf("list dropped %s", name)
					}
				}
				if _, exists := page.Requests[0].Evidence["requested_service_tier"]; exists {
					t.Fatal("list leaked full diagnostics")
				}
				var exported bytes.Buffer
				if err := c.ExportLogs(t.Context(), logbook.Query{}, &exported); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(exported.String(), `requested_service_tier`) || strings.Contains(exported.String(), "DO_NOT_STORE") || strings.Contains(f.logs.String(), "DO_NOT_STORE") || strings.Contains(exported.String(), "sk-test-only") {
					t.Fatal("tier export missing or arbitrary metadata leaked")
				}
			})
		}
	}
}

func TestServiceTierDiagnosticsOnConversionFailure(t *testing.T) {
	f := setup(t)
	p := state.Provider{ID: "p", Protocol: "anthropic"}
	r := httptest.NewRequest("POST", "/v1/responses", nil)
	tr := &requestTrace{row: logbook.Request{Evidence: map[string]string{"upstream_service_tier": "priority", "upstream_service_tier_raw": `"priority"`, "sent_service_tier_raw": `"priority"`, "fast_triggered": "true", "requested_service_tier": "ultrafast"}}}
	r = r.WithContext(context.WithValue(r.Context(), requestLogKey{}, tr))
	fields := map[string]json.RawMessage{"model": json.RawMessage(`"public"`), "input": json.RawMessage(`"hi"`), "service_tier": json.RawMessage(`"ultrafast"`)}
	f.core.forward(httptest.NewRecorder(), r, fields, "public", state.Candidate{Model: "m"}, p, state.ProviderKey{Protocol: "anthropic"}, false, state.Model{}, "")
	if _, exists := tr.row.Evidence["upstream_service_tier"]; exists {
		t.Fatal("previous candidate tier retained")
	}
	if tr.row.Evidence["upstream_service_tier_raw"] != "" || tr.row.Evidence["sent_service_tier_raw"] != "" || tr.row.Evidence["sent_service_tier"] != "not_sent" || tr.row.Evidence["fast_triggered"] != "unknown" {
		t.Fatalf("stale candidate data: %+v", tr.row.Evidence)
	}
	if tr.row.Evidence["requested_service_tier"] != "ultrafast" || tr.row.AttemptCount != 0 {
		t.Fatalf("%+v", tr.row)
	}
	setLogMetrics(tr, protocol.Observation{ServiceTier: "default"}, protocol.Usage{}, "responses", false)
	if tr.row.Evidence["upstream_service_tier"] != "default" {
		t.Fatal("failed upstream tier lost")
	}
}

func TestServiceTierLogsFailedGeneration(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[stream], func(t *testing.T) {
			f := setup(t)
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := `{"id":"r","status":"failed","service_tier":"default"}`
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					io.WriteString(w, "data: {\"type\":\"response.failed\",\"response\":"+body+"}\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, body)
				}
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "test")
			f.model(t, state.Candidate{ProviderID: "p", Model: "m"})
			body := `{"model":"public","input":"hi","service_tier":"priority","stream":` + map[bool]string{false: "false", true: "true"}[stream] + `}`
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, body)
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if err := f.core.logs.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			row, err := logClient(t, f).LogDetail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
			if err != nil || row.Status != "failed" || row.Evidence["requested_service_tier"] != "priority" || row.Evidence["upstream_service_tier"] != "default" {
				t.Fatalf("%+v %v", row, err)
			}
		})
	}
}

func TestProbeServiceTierRawAndCredentialProtection(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, tier := range []string{"ultrafast", "synthetic-upstream-secret"} {
			t.Run(tier+map[bool]string{false: "/json", true: "/sse"}[stream], func(t *testing.T) {
				f := setup(t)
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body := `{"id":"r","status":"completed","service_tier":"` + tier + `","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\ndata: {\"type\":\"response.completed\",\"response\":"+body+"}\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						io.WriteString(w, body)
					}
				}))
				defer up.Close()
				f.add(t, "p", up.URL, "synthetic-upstream-secret")
				provider, _ := f.store.Provider("p")
				var notify func(string, string)
				if stream {
					notify = func(string, string) {}
				}
				result, err := f.core.probeObserved(t.Context(), provider, "default", "m", notify)
				if err != nil || result.Failure != "" {
					t.Fatal(result, err)
				}
				if err := f.core.logs.Flush(t.Context()); err != nil {
					t.Fatal(err)
				}
				page, err := logClient(t, f).Logs(t.Context(), logbook.Query{Source: "probe"})
				if err != nil || len(page.Requests) != 1 {
					t.Fatal(page, err)
				}
				e := page.Requests[0].Evidence
				wantRaw, wantFast := `"ultrafast"`, "true"
				if tier != "ultrafast" {
					wantRaw, wantFast = "", "unknown"
				}
				if e["sent_service_tier"] != "unspecified" || e["upstream_service_tier_raw"] != wantRaw || e["fast_triggered"] != wantFast {
					t.Fatalf("%+v", e)
				}
			})
		}
	}
}

func TestRequestedServiceTierCredentialProtection(t *testing.T) {
	for _, header := range []string{"Authorization", "x-api-key"} {
		t.Run(header, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/v1/responses", nil)
			const secret = "synthetic-inbound-credential"
			value := secret
			if header == "Authorization" {
				value = "Bearer " + value
			}
			r.Header.Set(header, value)
			tr := &requestTrace{row: logbook.Request{Evidence: map[string]string{}}}
			r = r.WithContext(context.WithValue(r.Context(), requestLogKey{}, tr))
			observeRequestedServiceTier(r, map[string]json.RawMessage{"service_tier": json.RawMessage(`"` + secret + `"`)})
			if tr.row.Evidence["requested_service_tier_raw"] != "" || tr.row.Evidence["requested_service_tier"] != "unknown" {
				t.Fatal("credential retained", tr.row.Evidence)
			}
		})
	}
}
