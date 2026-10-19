package core

import (
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestDegradedKeyWeightedShareAndFallback(t *testing.T) {
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
			f.core.finishKey(p, "default", 401, "", false)
			counts := map[string]int{}
			for i := 0; i < 110; i++ {
				k, ok := f.core.chooseKey(p, nil)
				if !ok {
					t.Fatal("no Key")
				}
				counts[k.ID]++
			}
			if counts["default"] < 9 || counts["default"] > 11 || counts["second"] < 99 {
				t.Fatal(strategy, counts)
			}
			// The degraded Key is still eligible when healthy Keys have been tried.
			if k, ok := f.core.chooseKey(p, map[string]bool{"second": true}); !ok || k.ID != "default" {
				t.Fatal(k, ok)
			}
			f.core.finishKey(p, "second", 429, "", false)
			if _, ok := f.core.chooseKey(p, nil); !ok {
				t.Fatal("all degraded excluded")
			}
			f.core.finishKey(p, "default", 200, "", true)
			if f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true}).State != "ready" {
				t.Fatal("success did not restore")
			}
			// A live policy update changes the actual share and clears old penalties.
			enabled, threshold, seconds, weight := true, 3, 30, 25
			in := state.SaveKeyCooldownSettings{Enabled: &enabled, WeightPercent: &weight, FailureThreshold: &threshold, CooldownSeconds: &seconds, RateLimitSeconds: &seconds, ExpectedRevision: 1}
			if _, err := f.core.saveKeyCooldownSettings(in); err != nil {
				t.Fatal(err)
			}
			f.core.finishKey(p, "default", 401, "", false)
			count := 0
			for i := 0; i < 100; i++ {
				k, ok := f.core.chooseKey(p, nil)
				if !ok {
					t.Fatal("no Key")
				}
				if k.ID == "default" {
					count++
				}
			}
			want := 25
			if strategy == "round-robin" {
				want = 20
			}
			if count != want {
				t.Fatal("configured share", count, want)
			}
			weight = 100
			in.ExpectedRevision = 2
			if _, err := f.core.saveKeyCooldownSettings(in); err != nil {
				t.Fatal(err)
			}
			f.core.finishKey(p, "default", 401, "", false)
			if f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true}).State != "ready" {
				t.Fatal("100 percent penalized Key")
			}
		})
	}
}

func TestModelStrategiesDownweightWithoutExclusion(t *testing.T) {
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
			f.core.rememberRoute(m, m.Candidates[0], a, "default", "affinity", time.Millisecond)
			f.core.rememberRoute(m, m.Candidates[1], b, "default", "other", time.Second)
			count := 0
			for i := 0; i < 110; i++ {
				routes, err := f.core.routeCandidatesFor(m, "affinity")
				if err != nil || len(routes) != 2 {
					t.Fatal(routes, err)
				}
				if routes[0].ProviderID == "a" {
					count++
				}
			}
			if count < 9 || count > 11 {
				t.Fatalf("%s degraded first %d/110", strategy, count)
			}
			f.core.finishKey(b, "default", 429, "", false)
			routes, err := f.core.routeCandidatesFor(m, "affinity")
			if err != nil || len(routes) != 2 {
				t.Fatal("all degraded excluded", routes, err)
			}
			if err := f.store.MutateProvider("b", b.Revision, "key-put", "default", "", "", false, false); err != nil {
				t.Fatal(err)
			}
			routes, err = f.core.routeCandidatesFor(m, "affinity")
			if err != nil || len(routes) != 1 || routes[0].ProviderID != "a" {
				t.Fatal("manual disable bypassed", routes, err)
			}
		})
	}
}
