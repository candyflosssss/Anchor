package state

import (
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
)

func TestDeleteProviderKeyPrunesOnlyItsSources(t *testing.T) {
	for _, clearDefault := range []bool{false, true} {
		name := "key-delete"
		if clearDefault {
			name = "clear-default"
		}
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			secret := "synthetic-key"
			p, err := s.PutProvider(provider("p"), 0, &secret)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.PutProvider(provider("other"), 0, &secret); err != nil {
				t.Fatal(err)
			}
			// Even a disabled other Key must retain its own mappings.
			if err = s.MutateProvider(p.ID, p.Revision, "key-put", "backup", secret, "", false, false); err != nil {
				t.Fatal(err)
			}
			cases := []struct {
				id         string
				candidates []Candidate
				want       []Candidate
			}{
				{"only", []Candidate{{ProviderID: "p", Model: "same", KeyIDs: []string{"default"}}}, nil},
				{"shared-keys", []Candidate{{ProviderID: "p", Model: "same", KeyIDs: []string{"default", "backup"}}}, []Candidate{{ProviderID: "p", Model: "same", KeyIDs: []string{"backup"}}}},
				{"same-provider", []Candidate{{ProviderID: "p", Model: "same", KeyIDs: []string{"default"}}, {ProviderID: "p", Model: "other-up", KeyIDs: []string{"backup"}}}, []Candidate{{ProviderID: "p", Model: "other-up", KeyIDs: []string{"backup"}}}},
				{"other-provider", []Candidate{{ProviderID: "p", Model: "same", KeyIDs: []string{"default"}}, {ProviderID: "other", Model: "same", KeyIDs: []string{"default"}}}, []Candidate{{ProviderID: "other", Model: "same", KeyIDs: []string{"default"}}}},
				{"untouched", []Candidate{{ProviderID: "p", Model: "same", KeyIDs: []string{"backup"}}}, []Candidate{{ProviderID: "p", Model: "same", KeyIDs: []string{"backup"}}}},
				{"all-keys", []Candidate{{ProviderID: "p", Model: "same"}}, []Candidate{{ProviderID: "p", Model: "same"}}},
			}
			models := map[string]Model{}
			for _, c := range cases {
				m, err := s.PutModel(Model{ID: c.id, DisplayName: "Keep metadata", Strategy: "priority", Tools: true, Streaming: true, Candidates: c.candidates}, 0)
				if err != nil {
					t.Fatal(c.id, err)
				}
				models[c.id] = m
			}
			p, err = s.Provider(p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.PutPrice(p.ID, p.Revision, catalog.Price{Model: "other-provider", InputUSD: 1, OutputUSD: 2}); err != nil {
				t.Fatal(err)
			}
			if err = s.PutPrice("other", 1, catalog.Price{Model: "other-provider", InputUSD: 3, OutputUSD: 4}); err != nil {
				t.Fatal(err)
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			// An enabled default must not prevent removing one of several sources.
			if _, err = s.AddBinding(Binding{ID: "agent", AdapterID: "claude", AdapterVersion: "1", ConfigRoot: t.TempDir(), Executable: exe, Model: "shared-keys", Enabled: true}); err != nil {
				t.Fatal(err)
			}
			p, _ = s.Provider(p.ID)
			remove := func(revision int64) error {
				if clearDefault {
					empty := ""
					_, err := s.PutProvider(p, revision, &empty)
					return err
				}
				return s.MutateProvider(p.ID, revision, "key-delete", "default", "", "", false, false)
			}
			if err = remove(p.Revision - 1); !errors.Is(err, ErrConflict) {
				t.Fatal("stale revision accepted", err)
			}
			if err = remove(p.Revision); err != nil {
				t.Fatal(err)
			}
			for _, c := range cases {
				got, err := s.Model(c.id)
				if len(c.want) == 0 {
					if !errors.Is(err, ErrNotFound) {
						t.Fatalf("orphan %s retained: %+v, %v", c.id, got, err)
					}
					continue
				}
				want := models[c.id]
				if !reflect.DeepEqual(want.Candidates, c.want) {
					want.Revision++
				}
				want.Candidates = c.want
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("%s: got %+v (%v), want %+v", c.id, got, err, want)
				}
			}
			keys, err := s.ProviderKeys(p.ID)
			if err != nil || len(keys) != 1 || keys[0].ID != "backup" {
				t.Fatal(keys, err)
			}
			gotProvider, err := s.Provider(p.ID)
			if err != nil || gotProvider.Revision != p.Revision+1 {
				t.Fatal(gotProvider, err)
			}
			if prices, err := s.Prices(p.ID); err != nil || len(prices) != 0 {
				t.Fatal("removed candidate price retained", prices, err)
			}
			if prices, err := s.Prices("other"); err != nil || len(prices) != 1 {
				t.Fatal("other provider price removed", prices, err)
			}
		})
	}
}

func TestDeleteLastKeyRemovesAllKeysCandidate(t *testing.T) {
	s := newStore(t)
	secret := "synthetic"
	p, err := s.PutProvider(provider("p"), 0, &secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PutModel(Model{ID: "all-keys", Candidates: []Candidate{{ProviderID: p.ID, Model: "up"}}}, 0); err != nil {
		t.Fatal(err)
	}
	if err = s.MutateProvider(p.ID, p.Revision, "key-delete", "missing", "", "", false, false); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing Key accepted", err)
	}
	if err = s.MutateProvider(p.ID, p.Revision, "key-delete", "default", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Model("all-keys"); !errors.Is(err, ErrNotFound) {
		t.Fatal("last saved Key became an anonymous mapping", err)
	}
}

func TestDeleteKeyRollsBackWhenLastModelIsEnabledBindingDefault(t *testing.T) {
	for _, clearDefault := range []bool{false, true} {
		t.Run(map[bool]string{false: "key-delete", true: "clear-default"}[clearDefault], func(t *testing.T) {
			s := newStore(t)
			secret := "synthetic"
			p, err := s.PutProvider(provider("p"), 0, &secret)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"orphan", "bound"} {
				if _, err = s.PutModel(Model{ID: id, Candidates: []Candidate{{ProviderID: p.ID, Model: "up", KeyIDs: []string{"default"}}}}, 0); err != nil {
					t.Fatal(err)
				}
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			b, err := s.AddBinding(Binding{ID: "agent", AdapterID: "claude", AdapterVersion: "1", ConfigRoot: t.TempDir(), Executable: exe, Model: "bound", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			p, err = s.Provider(p.ID)
			if err != nil {
				t.Fatal(err)
			}
			remove := func() error {
				if clearDefault {
					empty := ""
					_, err := s.PutProvider(p, p.Revision, &empty)
					return err
				}
				return s.MutateProvider(p.ID, p.Revision, "key-delete", "default", "", "", false, false)
			}
			if err = remove(); !errors.Is(err, ErrReferenced) {
				t.Fatal("enabled default not protected", err)
			}
			if got, err := s.Provider(p.ID); err != nil || !reflect.DeepEqual(got, p) {
				t.Fatal("rejected deletion changed provider", got, err)
			}
			if keys, err := s.ProviderKeys(p.ID); err != nil || len(keys) != 1 || keys[0].Secret != secret {
				t.Fatal("rejected deletion removed Key", keys, err)
			}
			for _, id := range []string{"orphan", "bound"} {
				if m, err := s.Model(id); err != nil || m.Revision != 1 || !reflect.DeepEqual(m.Candidates[0].KeyIDs, []string{"default"}) {
					t.Fatal("rejected deletion changed model", m, err)
				}
			}
			if _, err = s.EnableBinding(b.ID, b.Revision, false); err != nil {
				t.Fatal(err)
			}
			if err = remove(); err != nil {
				t.Fatal("disabled binding blocked deletion", err)
			}
		})
	}
}
