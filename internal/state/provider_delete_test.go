package state

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
)

func TestDeleteProviderPrunesOnlyItsMappings(t *testing.T) {
	s := newStore(t)
	secret := "synthetic"
	for _, id := range []string{"p", "other"} {
		if _, err := s.PutProvider(provider(id), 0, &secret); err != nil {
			t.Fatal(err)
		}
	}
	shared, err := s.PutModel(Model{ID: "shared", Strategy: "priority", DisplayName: "Keep metadata", Tools: true, Candidates: []Candidate{
		{ProviderID: "p", Model: "same", KeyIDs: []string{"default"}},
		{ProviderID: "other", Model: "same", KeyIDs: []string{"default"}},
		{ProviderID: "p", Model: "another"},
	}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PutModel(Model{ID: "only", Candidates: []Candidate{{ProviderID: "p", Model: "same"}}}, 0); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddBinding(Binding{ID: "agent", AdapterID: "claude", AdapterVersion: "1", ConfigRoot: t.TempDir(), Executable: exe, Model: "shared", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"p", "other"} {
		if err = s.PutPrice(id, 1, catalog.Price{Model: "shared", InputUSD: 1, OutputUSD: 2}); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.MutateProvider("p", 1, "delete", "", "", "", false, false); !errors.Is(err, ErrConflict) {
		t.Fatal("stale deletion accepted", err)
	}
	if err = s.MutateProvider("p", 2, "delete", "", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Provider("p"); !errors.Is(err, ErrNotFound) {
		t.Fatal("provider retained", err)
	}
	if keys, err := s.ProviderKeys("p"); err != nil || len(keys) != 0 {
		t.Fatal(keys, err)
	}
	if _, err = s.Model("only"); !errors.Is(err, ErrNotFound) {
		t.Fatal("orphan retained", err)
	}
	want := shared
	want.Revision++
	want.Candidates = shared.Candidates[1:2]
	if got, err := s.Model("shared"); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("shared model changed", got, err)
	}
	if prices, err := s.Prices("other"); err != nil || len(prices) != 1 {
		t.Fatal("other price lost", prices, err)
	}
	if prices, err := s.Prices("p"); err != nil || len(prices) != 0 {
		t.Fatal("deleted price retained", prices, err)
	}
}
