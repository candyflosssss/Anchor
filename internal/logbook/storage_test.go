package logbook

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/modeldev"
)

func storageFixture(t *testing.T) Request {
	t.Helper()
	raw, err := os.ReadFile("../../desktop/contract/log-known-usage.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct{ Request Request }
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture.Request
}

func singleCostRequest(t *testing.T) Request {
	t.Helper()
	r := storageFixture(t)
	r.Attempts = r.Attempts[len(r.Attempts)-1:]
	r.AttemptCount, r.Attempts[0].Number = 1, 1
	r.AttemptsTruncated, r.KnownUsage = false, nil
	zero, free, empty := int64(0), "0", ""
	cost := &CostAdjustment{USD: &free, SupplierMultiplier: &free, Parts: []CostPart{
		{Kind: "input", Tokens: &zero, Rate: &free, USD: &free, FinalUSD: &free},
		{Kind: "output", Rate: &empty},
	}}
	r.ReferenceCost.Adjustment = cost
	r.Attempts[0].CostAdjustment = cost
	r.Metrics.CacheReadTokens = &zero
	r.ReasoningEffort = &empty
	r.Evidence["translation_category"] = ""
	r.fillKnownUsage()
	return r
}

func publicRequestJSON(t *testing.T, r Request) []byte {
	t.Helper()
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCompactRequestPreservesPublicJSON(t *testing.T) {
	for _, name := range []string{"empty", "single", "multiple", "different_cost", "missing_cost", "truncated", "unknown_count", "empty_collections"} {
		t.Run(name, func(t *testing.T) {
			r := singleCostRequest(t)
			switch name {
			case "empty":
				r = Request{}
			case "multiple":
				r = storageFixture(t)
			case "different_cost":
				r.Attempts[0].CostAdjustment = &CostAdjustment{Reason: "missing_usage"}
			case "missing_cost":
				r.Attempts[0].CostAdjustment = nil
			case "truncated":
				r.AttemptCount, r.AttemptsTruncated = 70, true
				r.KnownUsage = &KnownUsage{Input: 999999, Output: 888888, Attempts: 70}
			case "unknown_count":
				r.AttemptCount = 0
			case "empty_collections":
				r.Attempts = []Attempt{}
				r.KnownUsage = nil
				r.Evidence = map[string]string{}
				r.ReferenceCost.Adjustment.Parts = []CostPart{}
				r.ReferenceCost.Pricing = &modeldev.CatalogPricing{Tiers: []modeldev.CatalogTier{}}
			}
			r.fillKnownUsage()
			before := publicRequestJSON(t, r)
			raw, err := marshalStoredRequest(r)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, publicRequestJSON(t, r)) {
				t.Fatal("storage encoding changed the caller's request")
			}
			got, err := unmarshalStoredRequest(raw)
			if err != nil {
				t.Fatal(err)
			}
			after := publicRequestJSON(t, got)
			if !bytes.Equal(before, after) {
				t.Fatalf("public JSON changed\nbefore: %s\nafter: %s", before, after)
			}
			if bytes.Contains(after, []byte("_attempt_cost_from_root")) {
				t.Fatal("private storage marker escaped into public JSON")
			}
			if shared := bytes.Contains(raw, []byte("_attempt_cost_from_root")); shared != (name == "single") {
				t.Fatalf("unexpected cost reuse: %s", raw)
			}
			if name == "single" {
				if len(raw) >= len(before) {
					t.Fatalf("storage did not shrink: %d >= %d", len(raw), len(before))
				}
				*got.Attempts[0].CostAdjustment.Parts[0].Rate = "changed"
				if *got.ReferenceCost.Adjustment.Parts[0].Rate != "0" {
					t.Fatal("root and attempt snapshots share mutable state")
				}
				t.Logf("request JSON: %d -> %d bytes", len(before), len(raw))
			}
		})
	}
}

func TestCompactRequestPersistsAndReopens(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	r := singleCostRequest(t)
	r.StartedAtMS = time.Now().UnixMilli()
	want := publicRequestJSON(t, r)
	if !s.Request(r) {
		t.Fatal("request rejected")
	}
	flushTest(t, s)
	var raw string
	if err := s.read.QueryRow(`SELECT document FROM requests WHERE id=?`, r.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(raw), []byte("_attempt_cost_from_root")) || bytes.Contains([]byte(raw), []byte(`"known_usage"`)) {
		t.Fatal("database did not use compact storage")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, list := range []bool{false, true} {
		got, err := s.Detail(t.Context(), r.ID)
		if list {
			page, e := s.List(t.Context(), Query{})
			if e != nil || len(page.Requests) != 1 {
				t.Fatalf("list after restart: %+v %v", page, e)
			}
			got, err = page.Requests[0], e
		}
		if err != nil || !bytes.Equal(want, publicRequestJSON(t, got)) {
			t.Fatalf("reopened record changed (list=%v): %+v %v", list, got, err)
		}
	}
}
