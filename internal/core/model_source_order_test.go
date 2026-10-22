package core

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestIndependentKeyOrderPersistenceAndScheduling(t *testing.T) {
	f := setup(t)
	for _, id := range []string{"a", "b"} {
		f.add(t, id, "http://127.0.0.1:1/v1", "synthetic")
	}
	if err := f.store.MutateProvider("a", 1, "key-put", "second", "synthetic-second", "", true, false); err != nil {
		t.Fatal(err)
	}
	m, err := f.store.PutModel(state.Model{ID: "public", DisplayName: "Named", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "a", Model: "up", Multiplier: 2}, {ProviderID: "b", Model: "up", KeyIDs: []string{"default"}}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	refs := []catalog.SourceRoute{{ProviderID: "a", Model: "up", KeyID: "second"}, {ProviderID: "b", Model: "up", KeyID: "default"}, {ProviderID: "a", Model: "up", KeyID: "default"}}
	cfg, _ := f.store.Settings()
	send := func(input catalog.ModelRouting, want int) {
		t.Helper()
		raw, _ := json.Marshal(input)
		req, _ := http.NewRequest("POST", f.control.URL+"/control/v1/models/public/routing", strings.NewReader(string(raw)))
		req.Header.Set("Authorization", "Bearer "+f.owner)
		req.Header.Set("X-Anchor-Instance", cfg.InstanceID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body := text(t, resp)
		if resp.StatusCode != want {
			t.Fatalf("%d: %s", resp.StatusCode, body)
		}
	}
	input := catalog.ModelRouting{ModelID: m.ID, ExpectedRevision: m.Revision, Strategy: "priority", Order: []catalog.RouteCandidate{{ProviderID: "a", Model: "up"}, {ProviderID: "b", Model: "up"}}, SourceOrder: refs}
	send(input, 200)
	got, _ := f.store.Model(m.ID)
	if !reflect.DeepEqual(got.SourceOrder, refs) || !reflect.DeepEqual(got.Candidates, m.Candidates) || !got.Tools || got.DisplayName != "Named" {
		t.Fatalf("metadata/sources changed: %+v", got)
	}
	routes, err := f.core.routeCandidates(got)
	if err != nil {
		t.Fatal(err)
	}
	actual := []catalog.SourceRoute{}
	for _, c := range routes {
		actual = append(actual, catalog.SourceRoute{ProviderID: c.ProviderID, Model: c.Model, KeyID: c.KeyIDs[0]})
	}
	if !reflect.DeepEqual(actual, refs) {
		t.Fatalf("Key schedule: %+v", actual)
	}
	// Cooling only the first Key falls through to the other provider, before
	// the sibling Key on a. Membership and saved order stay unchanged.
	a, _ := f.store.Provider("a")
	f.core.finishKey(a, "second", 401, "", false)
	routes, _ = f.core.routeCandidates(got)
	if len(routes) != 2 || routes[0].ProviderID != "b" || routes[1].ProviderID != "a" || routes[1].KeyIDs[0] != "default" {
		t.Fatal(routes)
	}
	send(input, 409)
	input.ExpectedRevision = got.Revision
	input.SourceOrder = append([]catalog.SourceRoute(nil), refs...)
	input.SourceOrder[1] = refs[0]
	send(input, 400)
	input.SourceOrder = refs[:2]
	send(input, 400)
	input.SourceOrder = append([]catalog.SourceRoute(nil), refs...)
	input.SourceOrder[0].KeyID = "unknown"
	send(input, 400)
	unchanged, _ := f.store.Model(m.ID)
	if !reflect.DeepEqual(unchanged, got) {
		t.Fatal("rejected permutation changed model")
	}
	// New wildcard Keys append and never displace existing source ranks.
	if err := f.store.MutateProvider("a", 2, "key-put", "new", "synthetic-new", "", true, false); err != nil {
		t.Fatal(err)
	}
	routes, _ = f.core.routeCandidates(got)
	if routes[len(routes)-1].KeyIDs[0] != "new" {
		t.Fatal(routes)
	}
	// Rename and Key deletion preserve the remaining independent order.
	if err := f.store.MutateProvider("a", 3, "rename", "renamed", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	renamed, _ := f.store.Model(m.ID)
	if renamed.SourceOrder[0].ProviderID != "renamed" || renamed.SourceOrder[1].ProviderID != "b" {
		t.Fatal(renamed.SourceOrder)
	}
	if err := f.store.MutateProvider("renamed", 4, "key-delete", "second", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	pruned, _ := f.store.Model(m.ID)
	if len(pruned.SourceOrder) != 2 || pruned.SourceOrder[0].ProviderID != "b" {
		t.Fatal(pruned.SourceOrder)
	}
}
