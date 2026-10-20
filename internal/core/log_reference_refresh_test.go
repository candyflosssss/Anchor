package core

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestCoreStartupPrefetchDoesNotBlockReadyAndStopsWithCore(t *testing.T) {
	store, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// Reserve two distinct loopback ports until configuration is complete.
	control, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	gateway, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close()
	if _, err = store.Initialize(control.Addr().String(), gateway.Addr().String()); err != nil {
		t.Fatal(err)
	}
	control.Close()
	gateway.Close()
	started, cancelled := make(chan struct{}), make(chan struct{})
	client := modeldev.NewWithTransport(modelCatalogTransport(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		close(cancelled)
		return nil, r.Context().Err()
	}))
	srv := NewWithModelCatalog(store, slog.New(slog.NewJSONHandler(io.Discard, nil)), client)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("core did not stop with its background download")
		}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("core startup did not prefetch the catalog")
	}
	for deadline := time.Now().Add(time.Second); !srv.ready.Load() && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	if !srv.ready.Load() {
		t.Fatal("catalog download blocked core readiness")
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("core shutdown did not cancel catalog download")
	}
}

func TestLogReferenceRefreshesSavedIdentityWithoutChangingSettings(t *testing.T) {
	f := setup(t)
	installDefaultsCatalog(t, f)
	saved := logPriceSnapshot(t, "vision-reasoner")
	saved.FetchedAt = time.Now().Add(-4 * time.Hour).Unix()
	oldRate := "99"
	saved.Models[0].Pricing.Rates.Input = &oldRate
	m := state.Model{ServiceTier: "priority", Settings: &state.ModelSettings{Reference: &saved}}
	if f.core.logReference(m) != nil {
		t.Fatal("used a saved price older than three hours")
	}
	if _, err := f.core.modelCatalog.Resolve(t.Context(), "lab/vision-reasoner"); err != nil {
		t.Fatal(err)
	}
	price := f.core.logReference(m)
	if price == nil || *price.Pricing.Rates.Input != "2.5" || price.CatalogVersion == saved.Version || price.Stale {
		t.Fatal("did not use updated price for explicit reference identity", price)
	}
	if saved.Models[0].Pricing.Rates.Input != &oldRate || saved.FetchedAt >= time.Now().Add(-3*time.Hour).Unix() {
		t.Fatal("logging changed saved model settings")
	}
	// Inference still refuses to infer a standard price for a speed variant.
	if f.core.logReference(state.Model{ServiceTier: "priority", Candidates: []state.Candidate{{Model: "vision-reasoner"}}}) != nil {
		t.Fatal("speed variant inferred a standard price")
	}
}

func TestSavedReferenceFallbackHasThreeHourLimit(t *testing.T) {
	f := setup(t) // unavailable/cold catalog
	for _, age := range []time.Duration{30 * time.Minute, 2 * time.Hour, modeldev.ReferenceTTL, 4 * time.Hour} {
		saved := logPriceSnapshot(t, "vision-reasoner")
		saved.FetchedAt = time.Now().Add(-age).Unix()
		saved.ExpiresAt = saved.FetchedAt + int64(modeldev.CacheTTL/time.Second)
		price := f.core.logReference(state.Model{Settings: &state.ModelSettings{Reference: &saved}})
		if (price != nil) != (age < modeldev.ReferenceTTL) {
			t.Fatal(age, price)
		}
		if price != nil && price.Stale != (age >= modeldev.CacheTTL) {
			t.Fatal("saved fallback lost stale flag", age, price)
		}
		// Old snapshots without expires_at still use the observed fetch age.
		saved.ExpiresAt = 0
		price = f.core.logReference(state.Model{Settings: &state.ModelSettings{Reference: &saved}})
		if price != nil && price.Stale != (age >= modeldev.CacheTTL) {
			t.Fatal("missing expiry concealed old saved price", age, price)
		}
	}
}
