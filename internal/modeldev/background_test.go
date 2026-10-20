package modeldev

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestBackgroundPrefetchHourlyRefreshRetryAndStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, calls, fail := catalogClient(t)
		defer c.Close()
		c.StartRefresh()
		c.StartRefresh() // idempotent; only one timer/worker owns this client
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatal("startup did not prefetch once", calls.Load())
		}
		original, err := c.ResolveCached("vision-reasoner")
		if err != nil || original.Stale {
			t.Fatal(original, err)
		}
		time.Sleep(CacheTTL - time.Second)
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatal("fresh cache repeatedly downloaded", calls.Load())
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if calls.Load() != 2 {
			t.Fatal("no hourly refresh", calls.Load())
		}
		fail.Store(true)
		time.Sleep(CacheTTL)
		synctest.Wait()
		stale, err := c.ResolveCached("vision-reasoner")
		if calls.Load() != 3 || err != nil || !stale.Stale || stale.FetchedAt <= original.FetchedAt {
			t.Fatal("failed refresh discarded recent prices", calls.Load(), stale, err)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if calls.Load() != 4 {
			t.Fatal("refresh failure was not retried", calls.Load())
		}
		fail.Store(false)
		time.Sleep(time.Minute)
		synctest.Wait()
		fresh, err := c.ResolveCached("vision-reasoner")
		if calls.Load() != 5 || err != nil || fresh.Stale {
			t.Fatal("background retry did not recover", calls.Load(), fresh, err)
		}
		c.Close()
		c.StartRefresh()
		time.Sleep(2 * CacheTTL)
		synctest.Wait()
		if calls.Load() != 5 {
			t.Fatal("closed client kept refreshing", calls.Load())
		}
	})
}

func TestMappingRefreshSoonChecksAgeAndCoalesces(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, calls, _ := catalogClient(t)
		defer c.Close()
		base := time.Now()
		var offset atomic.Int64
		c.now = func() time.Time { return base.Add(time.Duration(offset.Load())) }
		c.StartRefresh()
		synctest.Wait()
		for range 100 {
			c.RefreshSoon()
		}
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatal("mapping edits downloaded a still fresh catalog", calls.Load())
		}
		offset.Store(int64(CacheTTL))
		for range 100 {
			c.RefreshSoon()
		}
		synctest.Wait()
		if calls.Load() != 2 {
			t.Fatal("mapping change did not refresh due data once", calls.Load())
		}
	})
}

func TestCloseCancelsBackgroundDownload(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var started, cancelled atomic.Bool
		c := NewWithTransport(transportFunc(func(r *http.Request) (*http.Response, error) {
			started.Store(true)
			<-r.Context().Done()
			cancelled.Store(true)
			return nil, context.Canceled
		}))
		c.StartRefresh()
		synctest.Wait()
		if !started.Load() {
			t.Fatal("prefetch did not start")
		}
		c.Close()
		if !cancelled.Load() {
			t.Fatal("Close did not join the cancelled download")
		}
	})
}

func TestCachedReferenceAgeAndFailedRefresh(t *testing.T) {
	c, _, fail := catalogClient(t)
	base := time.Now()
	c.now = func() time.Time { return base }
	if _, err := c.Resolve(t.Context(), "vision-reasoner"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		age           time.Duration
		stale, usable bool
	}{
		{CacheTTL - time.Second, false, true},
		{CacheTTL, true, true},
		{ReferenceTTL - time.Nanosecond, true, true},
		{ReferenceTTL, true, false},
	} {
		c.now = func() time.Time { return base.Add(tc.age) }
		out, err := c.ResolveCached("vision-reasoner")
		if (err == nil) != tc.usable || (tc.usable && out.Stale != tc.stale) {
			t.Fatalf("age=%s: %+v %v", tc.age, out, err)
		}
	}
	c.now = func() time.Time { return base.Add(time.Minute) }
	fail.Store(true)
	if _, err := c.Query(t.Context(), ModelCatalogQuery{Refresh: true}); err != nil {
		t.Fatal(err)
	}
	out, err := c.ResolveCached("vision-reasoner")
	if err != nil || !out.Stale || out.FetchedAt != base.Unix() {
		t.Fatal("failed refresh lost the last successful price", out, err)
	}
}
