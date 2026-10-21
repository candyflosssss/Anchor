package core

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestKeyRateRefreshDueAndExpiry(t *testing.T) {
	now := time.Now().UTC()
	value := 0.0 // zero is a real observation, never a missing value
	stamp := func(age time.Duration) string { return now.Add(-age).Format(time.RFC3339Nano) }
	for _, tc := range []struct {
		name        string
		rate        *catalog.KeyRateCache
		due, usable bool
	}{
		{"unknown", nil, true, false},
		{"undated", &catalog.KeyRateCache{Multiplier: &value}, true, false},
		{"fresh", &catalog.KeyRateCache{Multiplier: &value, ObservedAt: stamp(59 * time.Minute)}, false, true},
		{"hourly", &catalog.KeyRateCache{Multiplier: &value, ObservedAt: stamp(time.Hour)}, true, true},
		{"grace", &catalog.KeyRateCache{Multiplier: &value, ObservedAt: stamp(3*time.Hour - time.Nanosecond)}, true, true},
		{"expired", &catalog.KeyRateCache{Multiplier: &value, ObservedAt: stamp(3 * time.Hour)}, true, false},
		{"failed backoff", &catalog.KeyRateCache{Multiplier: &value, ObservedAt: stamp(2 * time.Hour), AttemptedAt: stamp(59 * time.Second), Failure: "network"}, false, true},
		{"failed retry", &catalog.KeyRateCache{Multiplier: &value, ObservedAt: stamp(2 * time.Hour), AttemptedAt: stamp(time.Minute), Failure: "network"}, true, true},
		{"future", &catalog.KeyRateCache{Multiplier: &value, ObservedAt: stamp(-time.Hour)}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := keyRateDue(tc.rate, now); got != tc.due {
				t.Fatal("due", got)
			}
			if got := tc.rate.UsableAt(now); got != tc.usable {
				t.Fatal("usable", got)
			}
		})
	}
}

func installRateRefreshTransport(t *testing.T, f *fixture) (*atomic.Int32, *atomic.Bool) {
	t.Helper()
	p, _ := f.store.Provider("p")
	if p.ID != "" {
		if err := f.store.SaveProviderPlatformDetection("p", p.Revision, "sub2api", "", false); err != nil {
			t.Fatal(err)
		}
	}
	calls, fail := new(atomic.Int32), new(atomic.Bool)
	f.core.upstream = &http.Client{Transport: modelCatalogTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Path != "/v1/sub2api/billing" || r.Header.Get("Authorization") == "" {
			t.Errorf("unexpected rate request: %s", r.URL.Path)
		}
		status := http.StatusOK
		if fail.Load() {
			status = http.StatusServiceUnavailable
		}
		body := `{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":2,"resolved_rate_multiplier":2,"peak_rate_enabled":false,"effective_rate_multiplier":2}`
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	return calls, fail
}

func TestKeyRateMaintenanceReadsEachEnabledKeyAndPreservesFailedObservation(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1/v1", "synthetic-rate-first")
	if err := f.store.MutateProvider("p", 1, "key-put", "second", "synthetic-rate-second", "", true, false); err != nil {
		t.Fatal(err)
	}
	p, _ := f.store.Provider("p")
	if err := f.store.MutateProvider("p", p.Revision, "key-put", "disabled", "synthetic-rate-disabled", "", false, false); err != nil {
		t.Fatal(err)
	}
	calls, fail := installRateRefreshTransport(t, f)
	attempts := map[rateTarget]rateAttempt{}
	now := time.Now()
	f.core.refreshKeyRates(t.Context(), now, attempts)
	keys, err := f.store.ProviderKeys("p")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || len(attempts) != 2 {
		t.Fatal("did not query enabled keys serially", calls.Load(), attempts)
	}
	for _, key := range keys {
		if !key.Enabled {
			if key.Rate != nil {
				t.Fatal("queried disabled key")
			}
			continue
		}
		if key.Rate == nil || key.Rate.Multiplier == nil || *key.Rate.Multiplier != 2 {
			t.Fatal("missing rate", key.ID)
		}
	}
	for range 10 {
		f.core.refreshKeyRates(t.Context(), now.Add(time.Second), attempts)
	}
	if calls.Load() != 2 {
		t.Fatal("mapping changes repeated fresh queries", calls.Load())
	}
	fail.Store(true)
	f.core.refreshKeyRates(t.Context(), now.Add(61*time.Minute), attempts)
	if calls.Load() != 4 {
		t.Fatal("did not refresh hourly", calls.Load())
	}
	failed, _ := f.store.ProviderKeys("p")
	for i, key := range failed {
		if !key.Enabled {
			continue
		}
		if key.Rate.Failure == "" || *key.Rate.Multiplier != 2 || key.Rate.ObservedAt != keys[i].Rate.ObservedAt {
			t.Fatal("failure replaced last observation", key.ID)
		}
	}
	// Removing a provider prunes retry bookkeeping as well as stored data.
	p, _ = f.store.Provider("p")
	if err := f.store.MutateProvider("p", p.Revision, "delete", "", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	f.core.refreshKeyRates(t.Context(), now.Add(62*time.Minute), attempts)
	if len(attempts) != 0 {
		t.Fatal("deleted keys retained refresh state")
	}
}

func TestKeyRateWorkerCancellationAndNoDuplicateStart(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1/v1", "synthetic-rate")
	if err := f.store.SaveProviderPlatformDetection("p", 1, "sub2api", "", false); err != nil {
		t.Fatal(err)
	}
	started, cancelled := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	f.core.upstream = &http.Client{Transport: modelCatalogTransport(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-r.Context().Done()
		close(cancelled)
		return nil, r.Context().Err()
	})}
	f.core.keyRateRefresh.start(f.core)
	f.core.keyRateRefresh.start(f.core)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("startup did not query rates")
	}
	for range 100 {
		f.core.keyRateRefresh.notify()
	}
	f.core.Close()
	select {
	case <-cancelled:
	default:
		t.Fatal("Close did not cancel/join query")
	}
	if calls.Load() != 1 {
		t.Fatal("multiple workers queried the same key")
	}
}

func TestBusyRateQueryBacksOffWithoutDelayingInference(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1/v1", "synthetic-rate")
	calls, _ := installRateRefreshTransport(t, f)
	attempts := map[rateTarget]rateAttempt{}
	if !f.core.take("p", 1) {
		t.Fatal("fixture concurrency")
	}
	now := time.Now()
	f.core.refreshKeyRates(t.Context(), now, attempts)
	f.core.release("p")
	f.core.refreshKeyRates(t.Context(), now.Add(59*time.Second), attempts)
	if calls.Load() != 0 {
		t.Fatal("busy retry ignored one-minute backoff")
	}
	f.core.refreshKeyRates(t.Context(), now.Add(time.Minute), attempts)
	if calls.Load() != 1 {
		t.Fatal("busy source never retried")
	}
}

func TestExpiredGroupRateDoesNotEnterNewCostsOrRouting(t *testing.T) {
	value := 2.0
	p := state.Provider{CostMultiplier: .5}
	for _, age := range []time.Duration{time.Minute, 2 * time.Hour, 3 * time.Hour, 4 * time.Hour} {
		key := state.ProviderKey{Rate: &catalog.KeyRateCache{Multiplier: &value, ObservedAt: time.Now().Add(-age).UTC().Format(time.RFC3339Nano), Failure: "network"}}
		a := logCostFactors(p, key)
		usable := age < catalog.KeyRateMaxAge
		if (a.GroupMultiplier != nil) != usable || (p.EffectiveKeyMultiplier(key) != nil) != usable {
			t.Fatal("expired multiplier used", age, a)
		}
		if a.ObservedAt == "" || a.Failure != "network" {
			t.Fatal("lost observation evidence")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f := setup(t)
	calls, _ := installRateRefreshTransport(t, f)
	f.core.refreshKeyRates(ctx, time.Now(), map[rateTarget]rateAttempt{})
	if calls.Load() != 0 {
		t.Fatal("cancelled maintenance made requests")
	}
}
