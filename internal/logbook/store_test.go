package logbook

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func flushTest(t *testing.T, s *Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e := s.Flush(ctx); e != nil {
		t.Fatal(e)
	}
}
func row(id string) Request {
	return Request{ID: id, StartedAtMS: time.Now().UnixMilli(), Status: "succeeded", Source: "proxy", Model: "test", Provider: "local", Key: "test", Stage: "respond", Attempts: []Attempt{}, Evidence: map[string]string{}}
}
func TestHistoryNullsPaginationAndSettings(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	for i := 0; i < 8; i++ {
		s.Request(row(fmt.Sprintf("req_%02d", i)))
	}
	flushTest(t, s)
	page, e := s.List(ctx, Query{Limit: 3})
	if e != nil || len(page.Requests) != 3 || !page.HasMore {
		t.Fatalf("page: %+v %v", page, e)
	}
	first := page.Requests[0]
	if first.Input != nil || first.Metrics.CacheReadTokens != nil || first.Stream != nil {
		t.Fatal("missing observations became zero")
	}
	next, e := s.List(ctx, Query{Limit: 3, Cursor: page.NextCursor, Until: page.Latest})
	if e != nil || len(next.Requests) != 3 || next.Requests[0].ID == first.ID {
		t.Fatalf("next: %+v %v", next, e)
	}
	found, e := s.List(ctx, Query{Search: "req_07"})
	if e != nil || len(found.Requests) != 1 {
		t.Fatalf("indexed search %+v %v", found, e)
	}
	cfg := s.Settings()
	cfg.Days = 14
	if e = s.Configure(ctx, cfg); e != nil || s.Settings().Days != 14 {
		t.Fatal(e)
	}
	r, e := s.Detail(ctx, first.ID)
	if e != nil || r.ID != first.ID {
		t.Fatal(e)
	}
}
func TestRecoverAndKeepTerminalOrdering(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	r := row("pending")
	r.Status = "pending"
	s.Request(r)
	flushTest(t, s)
	s.Request(row("done"))
	flushTest(t, s)
	page, _ := s.List(context.Background(), Query{})
	s.Close()
	s, e = Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	recovered, e := s.Detail(context.Background(), "pending")
	if e != nil || recovered.Status != "interrupted" || recovered.Code != "core_interrupted" {
		t.Fatalf("%+v %v", recovered, e)
	}
	cfg := s.Settings()
	if cfg != DefaultSettings() {
		t.Fatal(cfg)
	}
	recovered.Status = "failed"
	s.Request(recovered)
	flushTest(t, s)
	after, _ := s.List(context.Background(), Query{})
	if after.Latest != page.Latest || after.Requests[0].ID != "done" {
		t.Fatal("updates changed root sequence")
	}
}
func TestRetentionIsSeparateFromDisplayAndUsage(t *testing.T) {
	s := openTest(t)
	old := row("expired")
	old.StartedAtMS = time.Now().Add(-10 * 24 * time.Hour).UnixMilli()
	s.Request(old)
	s.Request(row("keep"))
	flushTest(t, s)
	if _, e := s.Detail(context.Background(), "expired"); e != ErrNotFound {
		t.Fatal(e)
	}
	if _, e := s.Detail(context.Background(), "keep"); e != nil {
		t.Fatal(e)
	}
	if s.Health().RequestCount != 1 {
		t.Fatal(s.Health())
	}
}
func TestOverflowNeverWaitsForSQLite(t *testing.T) {
	s := openTest(t)
	// Hold the sole writer connection while producers continue. Enqueue must remain bounded.
	conn, e := s.write.Conn(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	begin := time.Now()
	accepted := 0
	for i := 0; i < QueueSize*3; i++ {
		if s.Request(row(fmt.Sprintf("stress_%d", i))) {
			accepted++
		}
	}
	elapsed := time.Since(begin)
	conn.Close()
	flushTest(t, s)
	if s.Health().Dropped == 0 || accepted > QueueSize+BatchSize+1 {
		t.Fatalf("buffer unbounded %+v accepted=%d", s.Health(), accepted)
	}
	if elapsed > time.Second {
		t.Fatalf("producer waited on writer: %v", elapsed)
	}
}
func TestSystemEventsUseSafeAllowlist(t *testing.T) {
	s := openTest(t)
	l := slog.New(NewHandler(slog.NewTextHandler(io.Discard, nil), s))
	l.Info("core ready", "instance_id", "safe", "secret", "DO_NOT_STORE")
	l.Error("untrusted DO_NOT_STORE", "status", "DO_NOT_STORE")
	flushTest(t, s)
	p, e := s.List(context.Background(), Query{Kind: "system"})
	if e != nil || len(p.Events) != 1 || len(p.Events[0].Fields) != 1 || p.Events[0].Fields["instance_id"] != "safe" {
		t.Fatalf("%+v %v", p, e)
	}
}

func TestSubmittedSnapshotAndOversizeIsolation(t *testing.T) {
	s := openTest(t)
	conn, err := s.write.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r := row("snapshot")
	v := int64(10)
	httpStatus := 200
	r.Input = &v
	r.HTTP = &httpStatus
	r.Evidence["safe"] = "before"
	r.Attempts = []Attempt{{Number: 1, Input: &v}}
	if !s.Request(r) {
		t.Fatal("submission failed")
	}
	v = 99
	httpStatus = 500
	r.Evidence["safe"] = "after"
	r.Attempts[0].Number = 7
	large := row("oversize")
	large.Evidence["large"] = strings.Repeat("x", MaxRequestSize)
	if s.Request(large) {
		t.Fatal("oversized document accepted")
	}
	s.Request(row("after"))
	conn.Close()
	flushTest(t, s)
	got, err := s.Detail(context.Background(), "snapshot")
	if err != nil || *got.Input != 10 || *got.HTTP != 200 || got.Attempts[0].Number != 1 || got.Evidence["safe"] != "before" {
		t.Fatalf("snapshot mutated: %+v %v", got, err)
	}
	if _, err = s.Detail(context.Background(), "after"); err != nil {
		t.Fatal("oversize poisoned batch", err)
	}
}

// Opt-in substantial-history validation. Producers flush bounded chunks here so
// this test measures durable throughput, while the overflow test measures saturation.
func TestLargeHistoryPerformance(t *testing.T) {
	if os.Getenv("ANCHOR_LOG_PERF") != "1" {
		t.Skip("opt-in disk workload")
	}
	s := openTest(t)
	cfg := s.Settings()
	cfg.MaxRequests = 200000
	if err := s.Configure(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	for i := 0; i < 100000; i++ {
		r := row(fmt.Sprintf("history_%06d", i))
		r.Model = fmt.Sprintf("model_%d", i%100)
		if !s.Request(r) {
			t.Fatal("dropped paced record")
		}
		if i%512 == 511 {
			flushTest(t, s)
		}
	}
	flushTest(t, s)
	writeElapsed := time.Since(start)
	var plans []string
	rows, err := s.read.Query("EXPLAIN QUERY PLAN SELECT seq FROM requests WHERE model=? AND seq<? ORDER BY seq DESC LIMIT 51", "model_7", 99900)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a, b, c int
		var detail string
		if rows.Scan(&a, &b, &c, &detail) != nil {
			t.Fatal("explain scan")
		}
		plans = append(plans, detail)
	}
	rows.Close()
	if !strings.Contains(strings.Join(plans, " "), "request_model") {
		t.Fatal("query missed model index", plans)
	}
	var timings []time.Duration
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	before := mem.HeapAlloc
	for i := 0; i < 100; i++ {
		begin := time.Now()
		p, err := s.List(context.Background(), Query{Model: "model_7", Limit: 50, Cursor: 99900})
		if err != nil || len(p.Requests) != 50 {
			t.Fatalf("query %+v %v", p, err)
		}
		timings = append(timings, time.Since(begin))
	}
	slices.Sort(timings)
	runtime.ReadMemStats(&mem)
	var pages, limit, pageSize int64
	s.write.QueryRow("PRAGMA page_count").Scan(&pages)
	s.write.QueryRow("PRAGMA max_page_count").Scan(&limit)
	s.write.QueryRow("PRAGMA page_size").Scan(&pageSize)
	if pages > limit || limit*pageSize > 1<<30 || s.Health().Dropped != 0 {
		t.Fatal("physical/buffer limit failed", pages, limit, s.Health())
	}
	if timings[95] > 200*time.Millisecond {
		t.Fatalf("p95 query too slow: %v", timings[95])
	}
	t.Logf("100k durable records=%v; queries p50=%v p95=%v; db=%d MiB cap=%d MiB; heap_before=%d after=%d; queue=%d; plan=%v", writeElapsed, timings[50], timings[95], pages*pageSize>>20, limit*pageSize>>20, before, mem.HeapAlloc, s.Health().QueueDepth, plans)
	cfg.MaxRequests = 1000
	if err := s.Configure(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400; i++ {
		s.clean()
	}
	if s.Health().RequestCount > 1000 {
		t.Fatal("bounded cleanup failed", s.Health())
	}
	t.Logf("retention converged to %d records", s.Health().RequestCount)
}

func TestRetentionFailureIsVisible(t *testing.T) {
	s := openTest(t)
	s.Request(row("durable"))
	flushTest(t, s)
	// Simulate a storage failure after a successful write. Configuration must
	// report that cleanup failed and expose degradation without inventing drops.
	if _, err := s.write.Exec(`DROP TABLE events`); err != nil {
		t.Fatal(err)
	}
	if err := s.Configure(t.Context(), s.Settings()); err == nil {
		t.Fatal("cleanup failure was hidden")
	}
	h := s.Health()
	if h.WriteFailures == 0 || h.Error != "retention_failed" || h.Dropped != 0 {
		t.Fatal(h)
	}
	if _, err := s.Detail(t.Context(), "durable"); err != nil {
		t.Fatal(err)
	}
}
