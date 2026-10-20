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
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
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
	now := time.Now().Unix()
	return modeldev.ModelCatalog{Source: "models.dev", Version: strings.Repeat("a", 64), FetchedAt: now, ExpiresAt: now + int64(modeldev.CacheTTL/time.Second), Models: []modeldev.CatalogModel{m}, Total: 1}
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
			provider, _ := f.store.Provider("p")
			provider.CostMultiplier = .5
			provider, err := f.store.PutProvider(provider, provider.Revision, nil)
			if err != nil {
				t.Fatal(err)
			}
			multiplier := .8
			if err := f.store.CacheKeyRate("p", provider.Revision, "default", catalog.KeyRateCache{Multiplier: &multiplier, Group: "vip", Basis: "key_billing"}); err != nil {
				t.Fatal(err)
			}
			snapshot := logPriceSnapshot(t, "vision-reasoner")
			_, err = f.store.PutModel(state.Model{ID: "public", Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "actual-model"}}, Settings: &state.ModelSettings{Reference: &snapshot}}, 0)
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
			// Replacing both pricing and factors must not reprice history.
			provider.CostMultiplier = 2
			if _, err = f.store.PutProvider(provider, provider.Revision, nil); err != nil {
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
			if row.ReferenceCost.Adjustment == nil || row.ReferenceCost.Adjustment.USD == nil || *row.ReferenceCost.Adjustment.USD != "0.00104" || row.Attempts[0].CostAdjustment == nil || *row.Attempts[0].CostAdjustment.SupplierMultiplier != "0.5" {
				t.Fatal("adjustment snapshot lost", row.ReferenceCost)
			}
			page, err := cl.Logs(t.Context(), logbook.Query{})
			if err != nil || len(page.Requests) != 1 || page.Requests[0].ReferenceCost == nil || page.Requests[0].ReferenceCost.Adjustment == nil || len(page.Requests[0].ReferenceCost.Adjustment.Parts) != 5 || len(page.Requests[0].Attempts) != 0 {
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
	// A probe without cached model prices must omit factors as well.
	if _, err := f.core.probe(t.Context(), p, "default", "no-reference-model"); err != nil {
		t.Fatal(err)
	}
	if err := f.core.logs.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err = f.core.logs.List(t.Context(), logbook.Query{Source: "probe"})
	if err != nil || len(page.Requests) != 2 {
		t.Fatal(page, err)
	}
	for _, r := range page.Requests {
		if r.Model == "no-reference-model" && (r.ReferenceCost != nil || len(r.Attempts) != 1 || r.Attempts[0].CostAdjustment != nil) {
			t.Fatal("unpriced probe invented factors", r)
		}
	}
	if f.core.logReference(state.Model{ServiceTier: "priority", Candidates: []state.Candidate{{Model: "vision-reasoner"}}}) != nil {
		t.Fatal("inferred standard price for speed variant")
	}
	if f.core.logReference(state.Model{Candidates: []state.Candidate{{Model: "vision-reasoner"}, {Model: "text-only"}}}) != nil {
		t.Fatal("mixed reference identities")
	}
}

func TestLogCostAdjustmentSnapshotsAndRetryTotals(t *testing.T) {
	snapshot := logPriceSnapshot(t, "vision-reasoner")
	u := protocol.Usage{Known: true, Input: 1000, Output: 100, CacheRead: 400, CacheWrite: 100, Reasoning: 30, CacheReadKnown: true, CacheWriteKnown: true, ReasoningKnown: true}
	group := .8
	p := state.Provider{CostMultiplier: .5}
	key := state.ProviderKey{Rate: &catalog.KeyRateCache{Multiplier: &group, Group: "vip", GroupName: "VIP", Basis: "key_billing", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Failure: "network"}}
	factors := logCostFactors(p, key)
	group = 3 // capture before dispatch, not completion
	tr := &requestTrace{row: logbook.Request{ReferenceCost: referenceCost(&snapshot)}}
	tr.addAttempt(logbook.Attempt{CostAdjustment: factors}, u)
	a := tr.row.ReferenceCost.Adjustment
	if a == nil || a.USD == nil || *a.USD != "0.00106" || a.GroupMultiplier == nil || *a.GroupMultiplier != "0.8" || a.Failure != "network" || a.ObservedAt == "" {
		t.Fatalf("bad snapshot: %+v", a)
	}
	want := []struct {
		tokens            int64
		raw, final, ratio string
	}{{500, "0.00125", "0.0005", "1"}, {400, "0.0001", "0.00004", "0.1"}, {100, "0.0003", "0.00012", "1.2"}, {70, "0.0007", "0.00028", "4"}, {30, "0.0003", "0.00012", "4"}}
	for i, w := range want {
		part := a.Parts[i]
		if part.Tokens == nil || *part.Tokens != w.tokens || part.USD == nil || *part.USD != w.raw || part.FinalUSD == nil || *part.FinalUSD != w.final || part.RateMultiplier == nil || *part.RateMultiplier != w.ratio {
			t.Fatalf("part %d: %+v", i, part)
		}
	}
	// A different retry uses its own factors; never apply the last key to root.
	tr.addAttempt(logbook.Attempt{CostAdjustment: logCostFactors(state.Provider{CostMultiplier: 1}, key)}, u)
	a = tr.row.ReferenceCost.Adjustment
	if a.USD == nil || *a.USD != "0.00901" || !a.SupplierMixed || !a.GroupMixed || a.SupplierMultiplier != nil || a.GroupMultiplier != nil || *a.Parts[0].FinalUSD != "0.00425" {
		t.Fatalf("bad retry total: %+v", a)
	}
	tr.addAttempt(logbook.Attempt{}, u)
	tr.addAttempt(logbook.Attempt{CostAdjustment: factors}, u)
	if a.USD != nil || a.Reason != "missing_multiplier" || tr.row.ReferenceCost.USD == nil {
		t.Fatal("unknown group must not erase original cost or be replaced by one", a)
	}
}

func TestLogCostAdjustmentUnknownZeroAndTiers(t *testing.T) {
	snapshot := logPriceSnapshot(t, "vision-reasoner")
	u := protocol.Usage{Known: true, Input: 100, Output: 10, CacheReadKnown: true, CacheWriteKnown: true}
	zero := 0.0
	tr := &requestTrace{row: logbook.Request{ReferenceCost: referenceCost(&snapshot)}}
	tr.addAttempt(logbook.Attempt{CostAdjustment: logCostFactors(state.Provider{}, state.ProviderKey{Rate: &catalog.KeyRateCache{Multiplier: &zero, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}})}, u)
	a := tr.row.ReferenceCost.Adjustment
	if a.USD == nil || *a.USD != "0" || *a.SupplierMultiplier != "1" || a.Parts[4].Tokens != nil || !a.Parts[3].IncludesReasoning {
		t.Fatalf("zero/legacy/inclusive output: %+v", a)
	}
	tr.addAttempt(logbook.Attempt{CostAdjustment: logCostFactors(state.Provider{}, state.ProviderKey{Rate: &catalog.KeyRateCache{Multiplier: &zero, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}})}, protocol.Usage{})
	if a.USD != nil || a.Reason != "missing_usage" {
		t.Fatal("zero factor cannot invent missing usage", a)
	}
	tr = &requestTrace{row: logbook.Request{ReferenceCost: referenceCost(&snapshot)}}
	one := 1.0
	for _, input := range []int64{200000, 250001} {
		u.Input = input
		tr.addAttempt(logbook.Attempt{CostAdjustment: logCostFactors(state.Provider{}, state.ProviderKey{Rate: &catalog.KeyRateCache{Multiplier: &one, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}})}, u)
	}
	a = tr.row.ReferenceCost.Adjustment
	if a.USD == nil || *a.USD != "1.750255" || !a.Parts[0].MixedRate || a.Parts[0].Rate != nil {
		t.Fatalf("tiers: %+v", a)
	}
	// Totals continue after bounded attempt history is exhausted.
	for range 64 {
		tr.addAttempt(logbook.Attempt{CostAdjustment: logCostFactors(state.Provider{}, state.ProviderKey{Rate: &catalog.KeyRateCache{Multiplier: &one, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}})}, u)
	}
	if tr.row.AttemptCount != 66 || !tr.row.AttemptsTruncated || len(tr.row.Attempts) != 64 || *a.USD != "81.760175" {
		t.Fatal("truncated attempts lost totals", tr.row.AttemptCount, *a.USD)
	}
	noPrice := &requestTrace{}
	noPrice.addAttempt(logbook.Attempt{CostAdjustment: logCostFactors(state.Provider{}, state.ProviderKey{})}, u)
	if noPrice.row.Attempts[0].CostAdjustment != nil {
		t.Fatal("no model price should skip cost")
	}
}
