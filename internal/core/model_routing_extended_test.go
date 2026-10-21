package core

import (
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRoutingControlAtomicPreservation(t *testing.T) {
	f := setup(t)
	for _, id := range []string{"a", "b"} {
		f.add(t, id, "http://127.0.0.1:1/v1", "synthetic")
	}
	m, err := f.store.PutModel(state.Model{ID: "public", DisplayName: "Named", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "a", Model: "up-a", Multiplier: 2, KeyIDs: []string{"default"}}, {ProviderID: "b", Model: "up-b", Multiplier: .5, KeyIDs: []string{"default"}}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := f.store.Settings()
	send := func(in catalog.ModelRouting, want int) {
		t.Helper()
		b, _ := json.Marshal(in)
		r, _ := http.NewRequest("POST", f.control.URL+"/control/v1/models/public/routing", strings.NewReader(string(b)))
		r.Header.Set("Authorization", "Bearer "+f.owner)
		r.Header.Set("X-Anchor-Instance", cfg.InstanceID)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		raw := text(t, resp)
		if resp.StatusCode != want {
			t.Fatalf("%d %s", resp.StatusCode, raw)
		}
	}
	in := catalog.ModelRouting{ModelID: "public", ExpectedRevision: 1, Strategy: "lowest-price", Order: []catalog.RouteCandidate{{ProviderID: "b", Model: "up-b"}, {ProviderID: "a", Model: "up-a"}}}
	send(in, 200)
	got, _ := f.store.Model(m.ID)
	want := m
	want.Revision++
	want.Strategy = in.Strategy
	want.Candidates = []state.Candidate{m.Candidates[1], m.Candidates[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata lost: %+v", got)
	}
	send(in, 409)
	in.ExpectedRevision = 2
	in.Order[1] = in.Order[0]
	send(in, 400)
	in.Order = in.Order[:1]
	send(in, 400)
	got, _ = f.store.Model(m.ID)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("invalid write changed model")
	}
}

func TestNewRoutingRulesAndKeyRestrictions(t *testing.T) {
	f := setup(t)
	var mu sync.Mutex
	var seen []string
	upstream := func(id string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			seen = append(seen, id+":"+r.Header.Get("Authorization"))
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"r","output":[]}`)
		}))
	}
	a, b := upstream("a"), upstream("b")
	defer a.Close()
	defer b.Close()
	f.add(t, "a", a.URL, "synthetic-forbidden")
	f.add(t, "b", b.URL, "synthetic-b")
	if err := f.store.MutateProvider("a", 1, "key-put", "source", "synthetic-source", "", true, false); err != nil {
		t.Fatal(err)
	}
	m, err := f.store.PutModel(state.Model{ID: "public", Strategy: "lowest-price", Candidates: []state.Candidate{{ProviderID: "a", Model: "a", Multiplier: 1, KeyIDs: []string{"source"}}, {ProviderID: "b", Model: "b", Multiplier: 2}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	send := func() {
		t.Helper()
		r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
		raw := text(t, r)
		if r.StatusCode != 200 {
			t.Fatal(raw)
		}
	}
	send()
	if len(seen) != 1 || seen[0] != "a:Bearer synthetic-source" {
		t.Fatal("source Key ignored", seen)
	}
	p, _ := f.store.Provider("a")
	f.core.finishKey(p, "source", 401, "", false)
	send()
	if seen[1] != "b:Bearer synthetic-b" {
		t.Fatal("restricted key fell through to forbidden default", seen)
	}
	if err := f.store.MutateProvider("a", 2, "key-put", "source", "", "", true, false); err != nil {
		t.Fatal(err)
	} // revision resets health
	p, _ = f.store.Provider("a")
	m.Strategy = "lowest-latency"
	m, err = f.store.PutModel(m, m.Revision)
	if err != nil {
		t.Fatal(err)
	}
	// Unknown sources learn once in saved order, then use successful EWMA.
	routes, _ := f.core.routeCandidates(m)
	if routes[0].ProviderID != "a" {
		t.Fatal(routes)
	}
	f.core.rememberRoute(m, m.Candidates[0], p, "source", "", 100*time.Millisecond)
	routes, _ = f.core.routeCandidates(m)
	if routes[0].ProviderID != "b" {
		t.Fatal("unknown source never sampled", routes)
	}
	pb, _ := f.store.Provider("b")
	f.core.rememberRoute(m, m.Candidates[1], pb, "default", "", 10*time.Millisecond)
	routes, _ = f.core.routeCandidates(m)
	if routes[0].ProviderID != "b" {
		t.Fatal("latency ranking ignored", routes)
	}
	f.core.rememberRoute(m, m.Candidates[1], pb, "default", "", 20*time.Millisecond)
	score := f.core.latency[latencyID(m.ID, m.Revision, m.Candidates[1], pb)]
	if score.ms != 12 {
		t.Fatal("EWMA", score)
	}
	f.core.mu.Lock()
	score.at = time.Now().Add(-31 * time.Minute)
	f.core.latency[latencyID(m.ID, m.Revision, m.Candidates[1], pb)] = score
	f.core.mu.Unlock()
	routes, _ = f.core.routeCandidates(m)
	if routes[0].ProviderID != "b" {
		t.Fatal("expired source not relearned")
	}
}

func TestCacheAffinityCallerIsolationExpiryAndKeyFailover(t *testing.T) {
	f := setup(t)
	for _, id := range []string{"a", "b"} {
		f.add(t, id, "http://127.0.0.1:1/v1", "synthetic")
	}
	if err := f.store.MutateProvider("b", 1, "key-put", "source", "synthetic-source", "", true, false); err != nil {
		t.Fatal(err)
	}
	m, err := f.store.PutModel(state.Model{ID: "public", Strategy: "cache-priority", Candidates: []state.Candidate{{ProviderID: "a", Model: "a"}, {ProviderID: "b", Model: "b", KeyIDs: []string{"source"}}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	affinity := routingAffinity("caller-one", m.ID, "session-one")
	other := routingAffinity("caller-two", m.ID, "session-one")
	if affinity == other || affinity == routingAffinity("caller-one", m.ID, "session-two") || routingAffinity("caller-one", m.ID, "") != "" {
		t.Fatal("session isolation")
	}
	p, _ := f.store.Provider("b")
	f.core.rememberRoute(m, m.Candidates[1], p, "source", affinity, time.Millisecond)
	routes, _ := f.core.routeCandidatesFor(m, affinity)
	if routes[0].ProviderID != "b" || f.core.preferredRouteKey(m, m.Candidates[1], p, affinity) != "source" {
		t.Fatal("affinity ignored")
	}
	routes, _ = f.core.routeCandidatesFor(m, other)
	if routes[0].ProviderID != "a" {
		t.Fatal("affinity leaked to another caller")
	}
	f.core.finishKey(p, "source", 403, "60", false)
	routes, _ = f.core.routeCandidatesFor(m, affinity)
	if len(routes) != 1 || routes[0].ProviderID != "a" {
		t.Fatal("degraded affinity blocked failover", routes)
	}
	f.core.mu.Lock()
	entry := f.core.affinity[affinity]
	entry.at = time.Now().Add(-11 * time.Minute)
	f.core.affinity[affinity] = entry
	f.core.mu.Unlock()
	if f.core.preferredRouteKey(m, m.Candidates[1], p, affinity) != "source" {
		t.Fatal("idle session affinity lost")
	}
	entry.at = time.Now()
	f.core.affinity[affinity] = entry
	p.Revision++
	if f.core.preferredRouteKey(m, m.Candidates[1], p, affinity) != "source" {
		t.Fatal("provider revision lost session affinity")
	}
}

func TestCacheRoutingRemembersActualSuccessfulRequests(t *testing.T) {
	f := setup(t)
	var mu sync.Mutex
	counts := map[string]int{}
	upstream := func(id string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			counts[id]++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"r","output":[]}`)
		}))
	}
	a, b := upstream("a"), upstream("b")
	defer a.Close()
	defer b.Close()
	f.add(t, "a", a.URL, "synthetic-a")
	f.add(t, "b", b.URL, "synthetic-b")
	if _, err := f.store.PutModel(state.Model{ID: "public", Strategy: "cache-priority", Candidates: []state.Candidate{{ProviderID: "a", Model: "a", KeyIDs: []string{"default"}}, {ProviderID: "b", Model: "b", KeyIDs: []string{"default"}}}}, 0); err != nil {
		t.Fatal(err)
	}
	send := func(token string) {
		t.Helper()
		resp := request(t, "POST", f.gateway.URL+"/v1/responses", token, `{"model":"public","input":[{"role":"user","content":"stable prompt"}]}`)
		body := text(t, resp)
		if resp.StatusCode != 200 {
			t.Fatal(body)
		}
	}
	for i := 0; i < 3; i++ {
		send(f.token)
	}
	if counts["a"] != 3 || counts["b"] != 0 {
		t.Fatal("successful affinity not learned", counts)
	}
	_, token, err := f.store.Grant("another-caller", "gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	send(token)
	if counts["a"] != 4 || counts["b"] != 0 {
		t.Fatal("caller isolation", counts)
	}
	p, _ := f.store.Provider("a")
	f.core.finishKey(p, "default", 403, "60", false)
	send(f.token)
	if counts["b"] != 1 {
		t.Fatal("failed preferred key prevented successful failover", counts)
	}
}
