package state

import (
	"math"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/providerapi"
)

func TestProviderCostAndKeyCacheLifecycle(t *testing.T) {
	s := newStore(t)
	secret := "synthetic-cache-key"
	p, err := s.PutProvider(Provider{ID: "p", BaseURL: "https://example.com/v1", Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 1}, 0, &secret)
	if err != nil || p.CostMultiplier != 1 {
		t.Fatal(p, err)
	}
	p.CostMultiplier = .5
	p, err = s.PutProvider(p, p.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	rate := 2.0
	if err = s.CacheKeyRate(p.ID, p.Revision, "default", catalog.KeyRateCache{Multiplier: &rate, Basis: "key_billing"}); err != nil {
		t.Fatal(err)
	}
	keys, _ := s.ProviderKeys(p.ID)
	first := *keys[0].Rate
	if first.ObservedAt == "" || *p.EffectiveKeyMultiplier(keys[0]) != 1 {
		t.Fatal(keys, p)
	}
	if err = s.CacheKeyRate(p.ID, p.Revision, "default", catalog.KeyRateCache{Failure: "network"}); err != nil {
		t.Fatal(err)
	}
	keys, _ = s.ProviderKeys(p.ID)
	if *keys[0].Rate.Multiplier != 2 || keys[0].Rate.ObservedAt != first.ObservedAt || keys[0].Rate.Failure != "network" {
		t.Fatal(keys)
	}
	before, _ := s.Provider(p.ID)
	if before.Revision != p.Revision {
		t.Fatal("cache changed configuration revision")
	}
	// Missing field from an older writer preserves the provider factor.
	p.CostMultiplier = 0
	p, err = s.PutProvider(p, p.Revision, nil)
	if err != nil || p.CostMultiplier != .5 {
		t.Fatal(p, err)
	}
	if err = s.CacheKeyRate(p.ID, p.Revision-1, "default", catalog.KeyRateCache{Multiplier: &rate}); err != ErrConflict {
		t.Fatal("late cache accepted", err)
	}
	// Independent second Key, including a valid zero rate.
	if err = s.MutateProvider(p.ID, p.Revision, "key-put", "free", "synthetic-free", "", true, false); err != nil {
		t.Fatal(err)
	}
	p, _ = s.Provider(p.ID)
	zero := 0.0
	if err = s.CacheKeyRate(p.ID, p.Revision, "free", catalog.KeyRateCache{Multiplier: &zero, Basis: "key_billing"}); err != nil {
		t.Fatal(err)
	}
	keys, _ = s.ProviderKeys(p.ID)
	if keys[1].ID != "free" || *p.EffectiveKeyMultiplier(keys[1]) != 0 {
		t.Fatal(keys)
	}
	// A restart retains observations, not process-local state.
	reopened, err := Open(s.Dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	keys, _ = reopened.ProviderKeys(p.ID)
	if keys[0].Rate == nil || *keys[0].Rate.Multiplier != 2 {
		t.Fatal(keys)
	}
	// Changing just the provider factor recalculates every Key immediately.
	p.CostMultiplier = .25
	p, err = s.PutProvider(p, p.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	keys, _ = s.ProviderKeys(p.ID)
	if *p.EffectiveKeyMultiplier(keys[0]) != .5 {
		t.Fatal(keys)
	}
	// Credential replacement must not inherit another upstream identity's group.
	if err = s.MutateProvider(p.ID, p.Revision, "key-put", "default", "synthetic-replacement", "", true, false); err != nil {
		t.Fatal(err)
	}
	keys, _ = s.ProviderKeys(p.ID)
	if keys[0].Rate != nil || keys[1].Rate == nil {
		t.Fatal(keys)
	}
	p, _ = s.Provider(p.ID)
	p.BaseURLs = []string{"https://other.example.com/v1"}
	p.BaseURL = p.BaseURLs[0]
	p, err = s.PutProvider(p, p.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	keys, _ = s.ProviderKeys(p.ID)
	if keys[1].Rate != nil {
		t.Fatal("root change retained rate")
	}
}

func TestUnknownRatesValidationAndPlatformInvalidation(t *testing.T) {
	s := newStore(t)
	secret := "synthetic-rate"
	p, err := s.PutProvider(Provider{ID: "p", BaseURL: "https://example.com", TimeoutSeconds: 10, MaxConcurrent: 1}, 0, &secret)
	if err != nil {
		t.Fatal(err)
	}
	if p.EffectiveKeyMultiplier(ProviderKey{}) != nil {
		t.Fatal("unknown converted to 1")
	}
	for _, cost := range []float64{-1, 1e-9, 1001, math.NaN(), math.Inf(1)} {
		check := p
		check.CostMultiplier = cost
		if check.Validate() == nil {
			t.Fatal(cost)
		}
	}
	for _, v := range []float64{-1, math.NaN(), math.Inf(1)} {
		if s.CacheKeyRate(p.ID, p.Revision, "default", catalog.KeyRateCache{Multiplier: &v}) != ErrInvalid {
			t.Fatal(v)
		}
	}
	if err = s.CacheKeyRate(p.ID, p.Revision, "default", catalog.KeyRateCache{Failure: "unknown"}); err != nil {
		t.Fatal(err)
	}
	keys, _ := s.ProviderKeys(p.ID)
	if keys[0].Rate.Multiplier != nil || keys[0].Rate.ObservedAt != "" {
		t.Fatal(keys)
	}
	v := 1.0
	_ = s.CacheKeyRate(p.ID, p.Revision, "default", catalog.KeyRateCache{Multiplier: &v, Basis: "key_billing"})
	_, err = s.ConfigureProviderPlatform(providerapi.Configure{ProviderID: p.ID, ExpectedRevision: p.Revision, Config: providerapi.Config{Platform: "sub2api", SiteURL: "https://site.example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	keys, _ = s.ProviderKeys(p.ID)
	if keys[0].Rate != nil {
		t.Fatal("platform change retained cache")
	}
}
