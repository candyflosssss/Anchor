package state

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
)

func TestModelDeletionOnlyProtectsEnabledBindings(t *testing.T) {
	for _, providerScoped := range []bool{false, true} {
		name := "whole-model"
		if providerScoped {
			name = "last-provider-candidate"
		}
		t.Run(name, func(t *testing.T) {
			for _, scope := range []string{"", "single", "all-compatible", "all-mapped"} {
				t.Run("scope="+scope, func(t *testing.T) {
					s := newStore(t)
					if _, err := s.PutProvider(provider("p"), 0, nil); err != nil {
						t.Fatal(err)
					}
					m, err := s.PutModel(Model{ID: "alias", Candidates: []Candidate{{ProviderID: "p", Model: "up"}}}, 0)
					if err != nil {
						t.Fatal(err)
					}
					if err = s.PutPrice("p", 1, catalog.Price{Model: m.ID, InputUSD: 1, OutputUSD: 2}); err != nil {
						t.Fatal(err)
					}
					exe, err := os.Executable()
					if err != nil {
						t.Fatal(err)
					}
					bindings := []Binding{}
					for _, id := range []string{"a-disabled", "b-active"} {
						b, err := s.AddBinding(Binding{ID: id, AdapterID: "codex", AdapterVersion: "1", ConfigRoot: t.TempDir(), Executable: exe, Model: m.ID, ModelScope: scope, CodexVersion: "0.159.3", Enabled: true})
						if err != nil {
							t.Fatal(err)
						}
						bindings = append(bindings, b)
					}
					bindings[0], err = s.EnableBinding(bindings[0].ID, bindings[0].Revision, false)
					if err != nil {
						t.Fatal(err)
					}
					remove := func(revision int64) error {
						if providerScoped {
							return s.DeleteProviderModel(m.ID, revision, "p", 2)
						}
						return s.DeleteModel(m.ID, revision)
					}
					if err = remove(m.Revision); !errors.Is(err, ErrReferenced) {
						t.Fatalf("active binding must still block deletion: %v", err)
					}
					if got, err := s.Model(m.ID); err != nil || !reflect.DeepEqual(got, m) {
						t.Fatalf("rejected deletion changed model: %+v, %v", got, err)
					}
					if prices, err := s.Prices("p"); err != nil || len(prices) != 1 {
						t.Fatalf("rejected deletion removed price: %+v, %v", prices, err)
					}
					bindings[1], err = s.EnableBinding(bindings[1].ID, bindings[1].Revision, false)
					if err != nil {
						t.Fatal(err)
					}
					if err = remove(m.Revision + 1); !errors.Is(err, ErrConflict) {
						t.Fatalf("stale revision accepted: %v", err)
					}
					if err = remove(m.Revision); err != nil {
						t.Fatalf("disabled bindings must not block deletion: %v", err)
					}
					if _, err = s.Model(m.ID); !errors.Is(err, ErrNotFound) {
						t.Fatalf("model not removed: %v", err)
					}
					if prices, err := s.Prices("p"); err != nil || len(prices) != 0 {
						t.Fatalf("orphan price retained: %+v, %v", prices, err)
					}
					for _, b := range bindings {
						if got, err := s.Binding(b.ID); err != nil || !reflect.DeepEqual(got, b) {
							t.Fatalf("deletion changed disabled binding: %+v, %v", got, err)
						}
					}
				})
			}
		})
	}
}

func TestEnableBindingRequiresExistingDefaultModel(t *testing.T) {
	s := newStore(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.AddBinding(Binding{ID: "disabled", AdapterID: "claude", AdapterVersion: "1", ConfigRoot: t.TempDir(), Executable: exe, Model: "removed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.EnableBinding(b.ID, b.Revision+1, true); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	if _, err = s.EnableBinding(b.ID, b.Revision, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing default model enabled: %v", err)
	}
	if got, err := s.Binding(b.ID); err != nil || !reflect.DeepEqual(got, b) {
		t.Fatalf("rejected enable changed binding: %+v, %v", got, err)
	}
	if _, err = s.BindingToken(b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rejected enable activated credential: %v", err)
	}
	// Disabling an already disabled/orphaned binding remains possible.
	b, err = s.EnableBinding(b.ID, b.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PutProvider(provider("p"), 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PutModel(Model{ID: b.Model, Candidates: []Candidate{{ProviderID: "p", Model: "up"}}}, 0); err != nil {
		t.Fatal(err)
	}
	enabled, err := s.EnableBinding(b.ID, b.Revision, true)
	if err != nil || !enabled.Enabled || enabled.Revision != b.Revision+1 {
		t.Fatalf("restored model cannot be enabled: %+v, %v", enabled, err)
	}
	if _, err = s.BindingToken(b.ID); err != nil {
		t.Fatal(err)
	}
}
