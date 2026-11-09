package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestGatewayCooldownQueuesAcrossProviders(t *testing.T) {
	for _, strategy := range []string{"priority", "cache-priority", "round-robin", "lowest-price", "lowest-latency"} {
		for _, ordered := range []bool{false, true} {
			name := strategy
			if ordered {
				name += "/source-order"
			}
			t.Run(name, func(t *testing.T) {
				f := setup(t)
				var mu sync.Mutex
				var seen []string
				failReady := false
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
					mu.Lock()
					seen = append(seen, key)
					fail := failReady
					mu.Unlock()
					if key == "ready-a" || (key == "ready-b" && fail) {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"id":"r","output":[]}`)
				}))
				defer up.Close()
				f.add(t, "a", up.URL, "cooling-a")
				f.add(t, "b", up.URL, "ready-b")
				if err := f.store.MutateProvider("a", 1, "key-put", "second", "ready-a", "", true, false); err != nil {
					t.Fatal(err)
				}
				m := state.Model{ID: "public", Strategy: strategy, Candidates: []state.Candidate{{ProviderID: "a", Model: "up"}, {ProviderID: "b", Model: "up"}}}
				if ordered {
					m.SourceOrder = []state.SourceRoute{{ProviderID: "a", Model: "up", KeyID: "default"}, {ProviderID: "a", Model: "up", KeyID: "second"}, {ProviderID: "b", Model: "up", KeyID: "default"}}
				}
				if _, err := f.store.PutModel(m, 0); err != nil {
					t.Fatal(err)
				}
				p, _ := f.store.Provider("a")
				f.core.finishKey(p, "default", 401, "", false)
				until := f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true}).RetryAt
				// Even a free route with existing affinity must remain in fallback.
				cacheTestRate(t, f, "a", "default", 0)
				cacheTestRate(t, f, "a", "second", 1)
				cacheTestRate(t, f, "b", "default", 2)
				affinity := routingAffinity(f.token+"\x00", "public", "session")
				f.core.rememberRoute(m, m.Candidates[0], p, "default", affinity, 1)
				send := func() {
					t.Helper()
					req, _ := http.NewRequest("POST", f.gateway.URL+"/v1/responses", strings.NewReader(`{"model":"public","input":"hi"}`))
					req.Header.Set("Authorization", "Bearer "+f.token)
					req.Header.Set("X-Anchor-Session", "session")
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					body := text(t, resp)
					if resp.StatusCode != 200 {
						t.Fatal(resp.StatusCode, body)
					}
				}
				send()
				mu.Lock()
				first := append([]string(nil), seen...)
				seen, failReady = nil, true
				mu.Unlock()
				for _, k := range first {
					if k == "cooling-a" {
						t.Fatal("fallback used before a successful normal Key", first)
					}
				}
				// Reset only the ready sibling so both normal routes fail this time.
				f.core.mu.Lock()
				delete(f.core.health, healthID(p, "second"))
				f.core.mu.Unlock()
				send()
				mu.Lock()
				actual := append([]string(nil), seen...)
				mu.Unlock()
				if len(actual) != 3 || actual[2] != "cooling-a" || actual[0] == actual[1] {
					t.Fatal("normal queues were not exhausted before fallback", actual)
				}
				health := f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true})
				if !health.Enabled || health.State != "cooling" || health.RetryAt != until {
					t.Fatal("fallback changed cooldown or enabled state", health)
				}
			})
		}
	}
}

func TestGatewayAllCoolingKeysRemainEligibleAndScoped(t *testing.T) {
	for _, strategy := range []string{"priority", "round-robin"} {
		t.Run(strategy, func(t *testing.T) {
			f := setup(t)
			var mu sync.Mutex
			var seen []string
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				seen = append(seen, r.Header.Get("Authorization"))
				mu.Unlock()
				w.WriteHeader(http.StatusForbidden)
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "excluded")
			for i, id := range []string{"first", "second", "disabled"} {
				if err := f.store.MutateProvider("p", int64(i+1), "key-put", id, id, "", id != "disabled", false); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.store.MutateProvider("p", 4, "strategy", "", "", strategy, false, false); err != nil {
				t.Fatal(err)
			}
			p, _ := f.store.Provider("p")
			for _, id := range []string{"default", "first", "second", "disabled"} {
				f.core.finishKey(p, id, 401, "", false)
			}
			f.model(t, state.Candidate{ProviderID: "p", Model: "up", KeyIDs: []string{"first", "second", "disabled"}})
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
			body := text(t, resp)
			mu.Lock()
			actual := append([]string(nil), seen...)
			mu.Unlock()
			if resp.StatusCode != 503 || !reflect.DeepEqual(actual, []string{"Bearer first", "Bearer second"}) {
				t.Fatal("fallback skipped, replayed or broadened Key scope", resp.StatusCode, body, actual)
			}
		})
	}
}

func TestGatewaySingleCoolingKeyCanSucceed(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","output":[]}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	p, _ := f.store.Provider("p")
	f.core.finishKey(p, "default", 401, "", false)
	before := f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
	body := text(t, resp)
	if resp.StatusCode != 200 {
		t.Fatal("single cooling Key skipped", resp.StatusCode, body)
	}
	after := f.core.keyStatus(p, state.ProviderKey{ID: "default", Enabled: true})
	if !reflect.DeepEqual(before, after) {
		t.Fatal("successful fallback changed cooldown", before, after)
	}
}
