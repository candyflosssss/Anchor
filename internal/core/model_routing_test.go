package core

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestSharedProviderMappingActions(t *testing.T) {
	f := setup(t)
	for _, id := range []string{"a", "b"} {
		f.add(t, id, "http://127.0.0.1:1/v1", "synthetic")
	}
	cfg, _ := f.store.Settings()
	act := func(a catalog.Action, want int) catalog.Workspace {
		t.Helper()
		body, _ := json.Marshal(a)
		req, _ := http.NewRequest("POST", f.control.URL+"/control/v1/providers/"+a.ProviderID+"/action", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+f.owner)
		req.Header.Set("X-Anchor-Instance", cfg.InstanceID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != want {
			t.Fatalf("status=%d want=%d %s", resp.StatusCode, want, raw)
		}
		var out catalog.Workspace
		json.Unmarshal(raw, &out)
		return out
	}
	first := act(catalog.Action{ProviderID: "a", ExpectedRevision: 1, Action: "model-put", Target: "public", Model: "up-a", Tools: true, Streaming: true}, 200)
	if first.Models[0].Strategy != "round-robin" {
		t.Fatal(first)
	}
	second := act(catalog.Action{ProviderID: "b", ExpectedRevision: 1, Action: "model-put", Target: "public", Model: "up-b", Multiplier: 2, Strategy: "priority"}, 200)
	if second.Models[0].CandidateCount != 2 || !second.Models[0].Tools || !second.Models[0].Streaming || second.Models[0].Multiplier != 2 || second.Models[0].Strategy != "round-robin" {
		t.Fatal(second)
	}
	act(catalog.Action{ProviderID: "b", ExpectedRevision: 1, Action: "model-put", Target: "public", Model: "overwrite"}, 409)
	act(catalog.Action{ProviderID: "a", ExpectedRevision: 1, Action: "model-put", Target: "public", Model: "stale", ModelRevision: 1}, 409)
	act(catalog.Action{ProviderID: "a", ExpectedRevision: 1, Action: "model-put", Target: "public", Model: "new-a", ModelRevision: 2, Multiplier: .5, Tools: true, Streaming: true, Strategy: "priority"}, 200)
	m, _ := f.store.Model("public")
	if len(m.Candidates) != 2 || m.Candidates[1].Model != "up-b" || m.Candidates[1].Multiplier != 2 || m.Candidates[0].Multiplier != .5 || m.Strategy != "priority" {
		t.Fatal(m)
	}
	act(catalog.Action{ProviderID: "a", ExpectedRevision: 1, Action: "model-delete", Target: "public", ModelRevision: 3, Approved: true}, 200)
	m, _ = f.store.Model("public")
	if m.Revision != 4 || len(m.Candidates) != 1 || m.Candidates[0].ProviderID != "b" {
		t.Fatal(m)
	}
	act(catalog.Action{ProviderID: "b", ExpectedRevision: 1, Action: "model-put", Target: "public", Model: "up-b", ModelRevision: 4, Multiplier: -1}, 400)
	act(catalog.Action{ProviderID: "b", ExpectedRevision: 1, Action: "model-delete", Target: "public", ModelRevision: 4, Approved: true}, 200)
	if _, err := f.store.Model("public"); err != state.ErrNotFound {
		t.Fatal(err)
	}
}

func TestWeightedModelGatewayDistributionAndAvailability(t *testing.T) {
	f := setup(t)
	var a, b atomic.Int32
	upstream := func(counter *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counter.Add(1)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"id":"r","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)
		}))
	}
	ua, ub := upstream(&a), upstream(&b)
	defer ua.Close()
	defer ub.Close()
	f.add(t, "a", ua.URL, "synthetic-a")
	f.add(t, "b", ub.URL, "synthetic-b")
	cacheTestRate(t, f, "a", "default", 1)
	cacheTestRate(t, f, "b", "default", 2)
	m, err := f.store.PutModel(state.Model{ID: "public", Strategy: "round-robin", Candidates: []state.Candidate{{ProviderID: "a", Model: "up-a", Multiplier: 1}, {ProviderID: "b", Model: "up-b", Multiplier: 2}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	send := func() {
		t.Helper()
		r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
		raw := text(t, r)
		if r.StatusCode != 200 {
			t.Fatal(r.StatusCode, raw)
		}
	}
	for i := 0; i < 12; i++ {
		send()
	}
	if a.Load() != 8 || b.Load() != 4 {
		t.Fatalf("multiplier 1:2 should yield 8:4 requests, got %d:%d", a.Load(), b.Load())
	}
	pa, _ := f.store.Provider("a")
	f.core.finishKey(pa, "default", 429, "60", false)
	for i := 0; i < 3; i++ {
		send()
	}
	if a.Load() != 8 || b.Load() != 7 {
		t.Fatal("cooling candidate received traffic", a.Load(), b.Load())
	}
	f.core.mu.Lock()
	f.core.health[healthID(pa, "default")].Until = time.Now().Add(-time.Second)
	f.core.mu.Unlock()
	for i := 0; i < 3; i++ {
		send()
	}
	if a.Load() != 10 {
		t.Fatal("cooled candidate did not recover", a.Load())
	}
	if !f.core.take("a", 1) {
		t.Fatal("reserve")
	}
	send()
	f.core.release("a")
	if a.Load() != 10 {
		t.Fatal("busy candidate received traffic")
	}
	// Refreshing group caches changes weights immediately without model edits or routing debt.
	cacheTestRate(t, f, "a", "default", 2)
	cacheTestRate(t, f, "b", "default", 1)
	m.Candidates[0].Multiplier = 2
	m.Candidates[1].Multiplier = 1
	if _, err = f.store.PutModel(m, m.Revision); err != nil {
		t.Fatal(err)
	}
	beforeA, beforeB := a.Load(), b.Load()
	for i := 0; i < 6; i++ {
		send()
	}
	if a.Load()-beforeA != 2 || b.Load()-beforeB != 4 {
		t.Fatal("new revision weights not used", a.Load()-beforeA, b.Load()-beforeB)
	}
}

func TestWeightedCandidatesSkipUnavailableAndFailOver(t *testing.T) {
	f := setup(t)
	var failed, good atomic.Int32
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { failed.Add(1); w.WriteHeader(503) }))
	defer bad.Close()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		good.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","output":[]}`)
	}))
	defer ok.Close()
	f.add(t, "a", bad.URL, "synthetic")
	f.add(t, "b", ok.URL, "synthetic")
	m, err := f.store.PutModel(state.Model{ID: "public", Strategy: "round-robin", Candidates: []state.Candidate{{ProviderID: "a", Model: "a"}, {ProviderID: "b", Model: "b"}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
	raw := text(t, r)
	if r.StatusCode != 200 || failed.Load() != 1 || good.Load() != 1 {
		t.Fatal(r.StatusCode, raw, failed.Load(), good.Load())
	}
	p, _ := f.store.Provider("a")
	f.core.finishKey(p, "default", 401, "", false)
	routes, err := f.core.routeCandidates(m)
	if err != nil || len(routes) != 1 || routes[0].ProviderID != "b" {
		t.Fatal(routes, err)
	}
	if err = f.store.MutateProvider("b", 1, "key-put", "default", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	routes, err = f.core.routeCandidates(m)
	if err != nil || len(routes) != 0 {
		t.Fatal("disabled key scheduled", routes, err)
	}
	r = request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
	raw = text(t, r)
	if r.StatusCode != 503 || !strings.Contains(raw, "no_available_candidate") {
		t.Fatal(r.StatusCode, raw)
	}
}
