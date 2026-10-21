package logbook

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestKnownUsageSharedFixtureAndHistoricalReads(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/log-known-usage.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Request Request `json:"request"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	r := fixture.Request
	want := *r.KnownUsage
	r.KnownUsage = nil
	r.StartedAtMS = time.Now().UnixMilli()
	s := openTest(t)
	// Insert a pre-upgrade document directly so read-time compatibility is tested.
	raw, _ = json.Marshal(r)
	_, err = s.write.Exec(`INSERT INTO requests(id,created_at,status,source,model,provider,key_id,http,error_code,stage,size,document) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.StartedAtMS, r.Status, r.Source, r.Model, r.Provider, r.Key, r.HTTP, r.Code, r.Stage, len(raw), string(raw))
	if err != nil {
		t.Fatal(err)
	}
	detail, err := s.Detail(t.Context(), r.ID)
	if err != nil || detail.KnownUsage == nil || *detail.KnownUsage != want || detail.Input != nil || detail.Output != nil {
		t.Fatalf("detail: %+v %v", detail, err)
	}
	page, err := s.List(t.Context(), Query{})
	if err != nil || len(page.Requests) != 1 || page.Requests[0].KnownUsage == nil || *page.Requests[0].KnownUsage != want {
		t.Fatalf("list: %+v %v", page, err)
	}
	var stored string
	if err := s.read.QueryRow(`SELECT document FROM requests WHERE id=?`, r.ID).Scan(&stored); err != nil || stored != string(raw) {
		t.Fatal("historical read rewrote document", err)
	}
}

func TestKnownUsageMissingZeroAndTruncatedAttempts(t *testing.T) {
	zero, in, out, negative := int64(0), int64(12), int64(3), int64(-1)
	for _, tc := range []struct {
		name     string
		attempts []Attempt
		want     *KnownUsage
	}{
		{"missing", []Attempt{{}, {Input: &in}, {Output: &out}}, nil},
		{"zero", []Attempt{{}, {Input: &zero, Output: &zero}}, &KnownUsage{Attempts: 1}},
		{"mixed", []Attempt{{}, {Input: &in, Output: &out}, {Input: &negative, Output: &out}, {Input: &in, Output: &out}}, &KnownUsage{Input: 24, Output: 6, Attempts: 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Request{AttemptCount: 70, AttemptsTruncated: true, Attempts: tc.attempts}
			r.fillKnownUsage()
			if (r.KnownUsage == nil) != (tc.want == nil) || tc.want != nil && *r.KnownUsage != *tc.want {
				t.Fatalf("subtotal: %+v", r.KnownUsage)
			}
			if r.Input != nil || r.Output != nil {
				t.Fatal("partial usage became a root total")
			}
		})
	}
}
