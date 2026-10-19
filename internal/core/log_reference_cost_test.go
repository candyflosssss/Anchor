package core

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
)

func logPriceSnapshot(t *testing.T, id string) modeldev.ModelCatalog {
	t.Helper()
	raw, err := os.ReadFile("../modeldev/testdata/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := modeldev.ParseModel(raw, "lab", id)
	if err != nil {
		t.Fatal(err)
	}
	return modeldev.ModelCatalog{Source: "models.dev", Version: strings.Repeat("a", 64), FetchedAt: 1, ExpiresAt: 3601, Models: []modeldev.CatalogModel{m}, Total: 1}
}

func TestLogReferenceCostAttemptsAndMissingUsage(t *testing.T) {
	snapshot := logPriceSnapshot(t, "vision-reasoner")
	known := protocol.Usage{Known: true, Input: 1000, Output: 100, CacheRead: 400, CacheWrite: 100, Reasoning: 30, CacheReadKnown: true, CacheWriteKnown: true, ReasoningKnown: true}
	for _, tc := range []struct {
		name         string
		usage        protocol.Usage
		want, reason string
	}{
		{"inclusive cache and reasoning", known, "0.00265", ""},
		{"missing usage", protocol.Usage{}, "", "missing_usage"},
		{"missing cache write", protocol.Usage{Known: true, Input: 100, Output: 10, CacheReadKnown: true}, "", "missing_cache_usage"},
		{"missing cache read", protocol.Usage{Known: true, Input: 100, Output: 10, CacheWriteKnown: true}, "", "missing_cache_usage"},
		{"invalid usage", protocol.Usage{Known: true, Input: 1, Output: 10, CacheRead: 2, CacheReadKnown: true, CacheWriteKnown: true}, "", "invalid_usage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usd, reason := estimateLogAttempt(referenceCost(&snapshot), tc.usage)
			if reason != tc.reason || (tc.want == "" && usd != nil) || (tc.want != "" && (usd == nil || *usd != tc.want)) {
				t.Fatalf("usd=%v reason=%s", usd, reason)
			}
		})
	}
	tr := &requestTrace{row: logbook.Request{ReferenceCost: referenceCost(&snapshot)}}
	tr.addAttempt(logbook.Attempt{}, known)
	tr.addAttempt(logbook.Attempt{}, known)
	if got := tr.row.ReferenceCost.USD; got == nil || *got != "0.005300000000000000" {
		t.Fatal(tr.row.ReferenceCost)
	}
	if tr.row.Attempts[0].ReferenceUSD == nil || *tr.row.Attempts[0].ReferenceUSD != "0.00265" {
		t.Fatal(tr.row.Attempts)
	}
	tr.addAttempt(logbook.Attempt{}, protocol.Usage{})
	tr.addAttempt(logbook.Attempt{}, known)
	if tr.row.ReferenceCost.USD != nil || tr.row.ReferenceCost.Reason != "missing_usage" {
		t.Fatal("unknown retry understated total", tr.row.ReferenceCost)
	}
	// The tier is selected for each attempt, not from summed request input.
	tr = &requestTrace{row: logbook.Request{ReferenceCost: referenceCost(&snapshot)}}
	u := protocol.Usage{Known: true, Input: 200000, CacheReadKnown: true, CacheWriteKnown: true}
	tr.addAttempt(logbook.Attempt{}, u)
	tr.addAttempt(logbook.Attempt{}, u)
	if got := tr.row.ReferenceCost.USD; got == nil || *got != "1.000000000000000000" {
		t.Fatal(tr.row.ReferenceCost)
	}
	u.Input = 250001
	usd, reason := estimateLogAttempt(referenceCost(&snapshot), u)
	if reason != "" || usd == nil || *usd != "1.250005" {
		t.Fatal(usd, reason)
	}
	free := logPriceSnapshot(t, "text-only")
	freeUsage := known
	freeUsage.CacheRead, freeUsage.CacheWrite = 0, 0
	usd, reason = estimateLogAttempt(referenceCost(&free), freeUsage)
	if reason != "" || usd == nil || *usd != "0" {
		t.Fatal("free price lost", usd, reason)
	}
	snapshot.Models[0].Pricing = nil
	if referenceCost(&snapshot) != nil {
		t.Fatal("invented absent price")
	}
}

func TestRequestReferenceCostStoredAndExportedWithSnapshot(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "stream"}[stream], func(t *testing.T) {
			f := setup(t)
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := `{"id":"r","status":"completed","output":[],"usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":400},"output_tokens_details":{"reasoning_tokens":30}}}`
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					io.WriteString(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":"+response+"}\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, response)
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "synthetic-no-log-secret")
			snapshot := logPriceSnapshot(t, "vision-reasoner")
			_, err := f.store.PutModel(state.Model{ID: "public", Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "actual-model"}}, Settings: &state.ModelSettings{Reference: &snapshot}}, 0)
			if err != nil {
				t.Fatal(err)
			}
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, map[bool]string{false: `{"model":"public","input":"synthetic"}`, true: `{"model":"public","input":"synthetic","stream":true}`}[stream])
			io.Copy(io.Discard, resp.Body)
			if resp.StatusCode != 200 {
				t.Fatal(resp.StatusCode)
			}
			if err = f.core.logs.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			// Replacing the mapping does not change a persisted historical estimate.
			m, err := f.store.Model("public")
			if err != nil {
				t.Fatal(err)
			}
			m.Settings = nil
			if _, err = f.store.PutModel(m, m.Revision); err != nil {
				t.Fatal(err)
			}
			cl := logClient(t, f)
			row, err := cl.LogDetail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
			if err != nil || row.ReferenceCost == nil || row.ReferenceCost.USD == nil || *row.ReferenceCost.USD != "0.002600000000000000" || len(row.Attempts) != 1 || row.Attempts[0].ReferenceUSD == nil || row.ReferenceCost.CatalogVersion != snapshot.Version {
				t.Fatalf("row=%+v err=%v", row, err)
			}
			page, err := cl.Logs(t.Context(), logbook.Query{})
			if err != nil || len(page.Requests) != 1 || page.Requests[0].ReferenceCost == nil || len(page.Requests[0].Attempts) != 0 {
				t.Fatal(page, err)
			}
			var exported bytes.Buffer
			if err = cl.ExportLogs(t.Context(), logbook.Query{}, &exported); err != nil {
				t.Fatal(err)
			}
			var exportedRow logbook.Request
			if err = json.Unmarshal(exported.Bytes(), &exportedRow); err != nil || exportedRow.ReferenceCost == nil || exportedRow.ReferenceCost.USD == nil || *exportedRow.ReferenceCost.USD != *row.ReferenceCost.USD || strings.Contains(exported.String(), "synthetic-no-log-secret") {
				t.Fatal(exported.String(), err)
			}
		})
	}
}

func TestProbeReferenceCostUsesOnlyCachedCatalog(t *testing.T) {
	f := setup(t)
	installDefaultsCatalog(t, f)
	// Logging alone must not populate the catalog or start an HTTP request.
	if f.core.cachedLogReference("vision-reasoner") != nil {
		t.Fatal("cold cache invented price")
	}
	if _, err := f.core.modelCatalog.Resolve(t.Context(), "vision-reasoner"); err != nil {
		t.Fatal(err)
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":400}}}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	p, _ := f.store.Provider("p")
	if _, err := f.core.probe(t.Context(), p, "default", "vision-reasoner"); err != nil {
		t.Fatal(err)
	}
	if err := f.core.logs.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err := f.core.logs.List(t.Context(), logbook.Query{Source: "probe"})
	if err != nil || len(page.Requests) != 1 || page.Requests[0].ReferenceCost == nil || page.Requests[0].ReferenceCost.USD == nil || *page.Requests[0].ReferenceCost.USD != "0.0026" {
		t.Fatal(page, err)
	}
	if f.core.logReference(state.Model{ServiceTier: "priority", Candidates: []state.Candidate{{Model: "vision-reasoner"}}}) != nil {
		t.Fatal("inferred standard price for speed variant")
	}
	if f.core.logReference(state.Model{Candidates: []state.Candidate{{Model: "vision-reasoner"}, {Model: "text-only"}}}) != nil {
		t.Fatal("mixed reference identities")
	}
}
