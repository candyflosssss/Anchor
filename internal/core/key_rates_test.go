package core

import (
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

func cacheTestRate(t *testing.T, f *fixture, provider, key string, value float64) {
	t.Helper()
	p, err := f.store.Provider(provider)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.CacheKeyRate(provider, p.Revision, key, catalog.KeyRateCache{Multiplier: &value, Basis: "key_billing"}); err != nil {
		t.Fatal(err)
	}
}

func TestKeySpecificDerivedRouting(t *testing.T) {
	f := setup(t)
	f.add(t, "a", "http://127.0.0.1:1/v1", "synthetic-a")
	f.add(t, "b", "http://127.0.0.1:1/v1", "synthetic-b")
	if err := f.store.MutateProvider("a", 1, "key-put", "second", "synthetic-second", "", true, false); err != nil {
		t.Fatal(err)
	}
	p, _ := f.store.Provider("a")
	p.CostMultiplier = .5
	if _, err := f.store.PutProvider(p, p.Revision, nil); err != nil {
		t.Fatal(err)
	}
	cacheTestRate(t, f, "a", "default", 2) // effective 1
	cacheTestRate(t, f, "a", "second", .8) // effective .4
	cacheTestRate(t, f, "b", "default", 2) // effective 2
	m := state.Model{ID: "public", Strategy: "lowest-price", Candidates: []state.Candidate{{ProviderID: "a", Model: "up", Multiplier: 999}, {ProviderID: "b", Model: "up", Multiplier: .01}}}
	routes, err := f.core.routeCandidates(m)
	if err != nil || len(routes) != 3 || routes[0].ProviderID != "a" || routes[0].KeyIDs[0] != "second" || routes[1].KeyIDs[0] != "default" || routes[2].ProviderID != "b" {
		t.Fatal(routes, err)
	}
	// Equal provider/model, different Keys must be scheduled independently.
	m.Strategy = "round-robin"
	counts := map[string]int{}
	for i := 0; i < 80; i++ {
		r, e := f.core.routeCandidates(m)
		if e != nil {
			t.Fatal(e)
		}
		counts[r[0].ProviderID+"/"+r[0].KeyIDs[0]]++
	}
	// Weights 1 : 2.5 : .5, so 20 : 50 : 10 requests.
	if counts["a/default"] != 20 || counts["a/second"] != 50 || counts["b/default"] != 10 {
		t.Fatal(counts)
	}
	w, err := f.core.workspace("a")
	if err != nil || len(w.Keys) != 2 || *w.Keys[0].EffectiveMultiplier != 1 || *w.Keys[1].EffectiveMultiplier != .4 {
		t.Fatal(w, err)
	}
	// Restricting a source Key must not accidentally select a cheaper sibling.
	m.Candidates[0].KeyIDs = []string{"default"}
	m.Strategy = "lowest-price"
	routes, err = f.core.routeCandidates(m)
	if err != nil || len(routes) != 2 || routes[0].KeyIDs[0] != "default" {
		t.Fatal(routes, err)
	}
	// Fixed order retains its original provider/Key policy and ignores all factors.
	m.Strategy = "priority"
	routes, err = f.core.routeCandidates(m)
	if err != nil || len(routes) != 2 || routes[0].ProviderID != "a" {
		t.Fatal(routes, err)
	}
}

func TestRoutingUnknownZeroAndCachedFailure(t *testing.T) {
	f := setup(t)
	f.add(t, "a", "http://127.0.0.1:1/v1", "synthetic-a")
	f.add(t, "b", "http://127.0.0.1:1/v1", "synthetic-b")
	m := state.Model{ID: "public", Strategy: "lowest-price", Candidates: []state.Candidate{{ProviderID: "a", Model: "up"}, {ProviderID: "b", Model: "up"}}}
	cacheTestRate(t, f, "b", "default", 10)
	for _, strategy := range []string{"lowest-price", "round-robin"} {
		m.Strategy = strategy
		for i := 0; i < 6; i++ {
			r, e := f.core.routeCandidates(m)
			if e != nil || r[0].ProviderID != "b" || len(r) != 2 {
				t.Fatal(r, e)
			}
		}
	}
	cacheTestRate(t, f, "a", "default", 0)
	for _, strategy := range []string{"lowest-price", "round-robin"} {
		m.Strategy = strategy
		for i := 0; i < 6; i++ {
			r, e := f.core.routeCandidates(m)
			if e != nil || r[0].ProviderID != "a" {
				t.Fatal(r, e)
			}
		}
	}
	p, _ := f.store.Provider("a")
	_ = f.store.CacheKeyRate("a", p.Revision, "default", catalog.KeyRateCache{Failure: "network"})
	r, err := f.core.routeCandidates(m)
	if err != nil || r[0].ProviderID != "a" {
		t.Fatal(r, err)
	}
	// Health penalties retain free routes while favoring healthier unknown rates.
	f.core.finishKey(p, "default", 429, "60", false)
	r, err = f.core.routeCandidates(m)
	if err != nil || len(r) != 2 || r[0].ProviderID != "b" {
		t.Fatal(r, err)
	}
}
