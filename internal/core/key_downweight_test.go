package core

import (
	"github.com/candyflosssss/Anchor/internal/state"
	"testing"
	"time"
)

func TestKeyCooldownExcludesOnlyFailedKey(t *testing.T) {
	for _, strategy := range []string{"priority", "round-robin"} {
		t.Run(strategy, func(t *testing.T) {
			f := setup(t)
			f.add(t, "p", "http://127.0.0.1:1", "synthetic")
			if err := f.store.MutateProvider("p", 1, "key-put", "second", "synthetic-second", "", true, false); err != nil {
				t.Fatal(err)
			}
			p, _ := f.store.Provider("p")
			if err := f.store.MutateProvider("p", p.Revision, "strategy", "", "", strategy, false, false); err != nil {
				t.Fatal(err)
			}
			p, _ = f.store.Provider("p")
			f.core.finishKey(p, "default", 403, "", false)
			for i := 0; i < 20; i++ {
				k, ok := f.core.chooseKey(p, nil)
				if !ok || k.ID != "second" {
					t.Fatal(k, ok)
				}
			}
			if _, ok := f.core.chooseKey(p, map[string]bool{"second": true}); ok {
				t.Fatal("cooling Key selected")
			}
			f.core.finishKey(p, "second", 403, "", false)
			if _, ok := f.core.chooseKey(p, nil); ok {
				t.Fatal("all cooling Keys selected")
			}
			f.core.health[healthID(p, "default")].Until = time.Now().Add(-time.Second)
			for i := 0; i < 3; i++ {
				k, ok := f.core.chooseKey(p, nil)
				if !ok || k.ID != "default" {
					t.Fatal("expiry requires probe", k, ok)
				}
			}
		})
	}
}
func TestAllStrategiesKeepSessionsAndSkipCooldown(t *testing.T) {
	for _, strategy := range []string{"priority", "round-robin", "lowest-price", "lowest-latency", "cache-priority"} {
		t.Run(strategy, func(t *testing.T) {
			f := setup(t)
			for _, id := range []string{"a", "b"} {
				f.add(t, id, "http://127.0.0.1:1", "synthetic")
				cacheTestRate(t, f, id, "default", 1)
			}
			m := state.Model{ID: "public", Revision: 1, Strategy: strategy, Candidates: []state.Candidate{{ProviderID: "a", Model: "up"}, {ProviderID: "b", Model: "up"}}}
			a, _ := f.store.Provider("a")
			b, _ := f.store.Provider("b")
			f.core.finishKey(a, "default", 403, "", false)
			routes, err := f.core.routeCandidatesFor(m, "session")
			if err != nil || len(routes) != 1 || routes[0].ProviderID != "b" {
				t.Fatal(routes, err)
			}
			f.core.rememberRoute(m, m.Candidates[1], b, "default", "session", time.Millisecond)
			f.core.health[healthID(a, "default")].Until = time.Now().Add(-time.Second)
			m.Revision++
			routes, err = f.core.routeCandidatesFor(m, "session")
			if err != nil || len(routes) != 2 || routes[0].ProviderID != "b" {
				t.Fatal("session switched after recovery", routes, err)
			}
			if strategy == "priority" || strategy == "cache-priority" {
				routes, _ = f.core.routeCandidatesFor(m, "new-session")
				if routes[0].ProviderID != "a" {
					t.Fatal("new session ignored priority", routes)
				}
			}
			f.core.finishKey(b, "default", 403, "", false)
			routes, err = f.core.routeCandidatesFor(m, "session")
			if err != nil || len(routes) != 1 || routes[0].ProviderID != "a" {
				t.Fatal("session prevented fallback", routes, err)
			}
			f.core.finishKey(a, "default", 403, "", false)
			routes, err = f.core.routeCandidates(m)
			if err != nil || len(routes) != 0 {
				t.Fatal("all cooling routes selected", routes, err)
			}
		})
	}
}
func TestKeyErrorScoreExpiryAndClassification(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1", "synthetic")
	p, _ := f.store.Provider("p")
	for _, status := range []int{400, 404, 422, 200} {
		f.core.finishKey(p, "default", status, "", false)
	}
	h := f.core.health[healthID(p, "default")]
	if h.Score != 0 {
		t.Fatal(h)
	}
	f.core.finishKey(p, "default", 500, "", false)
	if h.Score != 25 || !h.Until.IsZero() {
		t.Fatal(h)
	}
	h.errors[0].until = time.Now().Add(-time.Second)
	f.core.finishKey(p, "default", 429, "", false)
	if h.Score != 50 || !h.Until.IsZero() {
		t.Fatal(h)
	}
	f.core.finishKey(p, "default", 200, "", true)
	if h.Score != 50 {
		t.Fatal("success erased window")
	}
	f.core.finishKey(p, "default", 429, "600", false)
	if h.Score != 100 || time.Until(h.Until) < 599*time.Second {
		t.Fatal(h)
	}
}
