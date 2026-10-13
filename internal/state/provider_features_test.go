package state

import (
	"encoding/json"
	"errors"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"os"
	"strings"
	"testing"
)

func TestPriorityProviderMutationsAndUsage(t *testing.T) {
	s := newStore(t)
	secret := "synthetic-default"
	p, err := s.PutProvider(provider("old"), 0, &secret)
	if err != nil {
		t.Fatal(err)
	}
	keys, _ := s.ProviderKeys(p.ID)
	if len(keys) != 1 || keys[0].ID != "default" {
		t.Fatal(keys)
	}
	if err = s.MutateProvider("old", 1, "key-put", "second", "synthetic-second", "", true, false); err != nil {
		t.Fatal(err)
	}
	if err = s.MutateProvider("old", 1, "key-delete", "default", "", "", false, false); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	m, err := s.PutModel(Model{ID: "alias", Candidates: []Candidate{{ProviderID: "old", Model: "up"}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MutateProvider("old", 2, "delete", "", "", "", false, false); !errors.Is(err, ErrReferenced) {
		t.Fatal(err)
	}
	if err = s.PutPrice("old", 2, catalog.Price{Model: "alias", InputUSD: 1, OutputUSD: 2}); err != nil {
		t.Fatal(err)
	}
	if err = s.MutateProvider("old", 3, "rename", "new", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	m, _ = s.Model(m.ID)
	if m.Revision != 2 || m.Candidates[0].ProviderID != "new" {
		t.Fatal(m)
	}
	keys, _ = s.ProviderKeys("new")
	b, _ := json.Marshal(keys)
	if len(keys) != 2 || strings.Contains(string(b), "synthetic") {
		t.Fatal("keys lost/leaked")
	}
	prices, _ := s.Prices("new")
	if len(prices) != 1 {
		t.Fatal(prices)
	}
	if err = s.MutateProvider("new", 4, "copy", "copy", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	cp, _ := s.Provider("copy")
	keys, _ = s.ProviderKeys("copy")
	if cp.Enabled || len(keys) > 0 {
		t.Fatal("copy not isolated")
	}
	if err = s.DeleteProviderModel("alias", m.Revision, "new", 3); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err = s.DeleteProviderModel("alias", m.Revision, "new", 4); err != nil {
		t.Fatal(err)
	}
	prices, _ = s.Prices("new")
	if len(prices) != 0 {
		t.Fatal("deleted model prices retained")
	}
	for _, u := range []Usage{{ProviderID: "new", Model: "alias", Requests: 1, Known: 1, InputTokens: 10}, {ProviderID: "new", Model: "alias", Requests: 1, Failures: 1}} {
		if err = s.RecordUsage(u); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.MutateProvider("new", 4, "delete", "", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	keys, _ = s.ProviderKeys("new")
	us, _ := s.Usage("new")
	if len(keys) != 0 || len(us) != 1 || us[0].Requests != 2 || us[0].Known != 1 || us[0].InputTokens != 10 {
		t.Fatal(keys, us)
	}
}
func TestPriorityV2KeyMigration(t *testing.T) {
	s := newStore(t)
	secret := "synthetic-legacy"
	if _, err := s.PutProvider(provider("p"), 0, &secret); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DROP TABLE provider_platforms; DROP TABLE key_model_protocols; DROP TABLE provider_keys; DROP TABLE provider_strategy; DROP TABLE provider_prices; DROP TABLE usage_totals; PRAGMA user_version=2;`); err != nil {
		t.Fatal(err)
	}
	dir := s.Dir
	s.Close()
	next, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	keys, err := next.ProviderKeys("p")
	if err != nil || len(keys) != 1 || keys[0].Secret != secret || !keys[0].Enabled {
		t.Fatal(keys, err)
	}
}

func TestSharedMappingRemovalPreservesBindingAndOtherPrices(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"a", "b"} {
		if _, err := s.PutProvider(provider(id), 0, nil); err != nil {
			t.Fatal(err)
		}
	}
	m, err := s.PutModel(Model{ID: "alias", Tools: true, Strategy: "round-robin", Candidates: []Candidate{{ProviderID: "a", Model: "up-a"}, {ProviderID: "b", Model: "up-b", Multiplier: 2}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Register an isolated synthetic binding to exercise the deletion reference guard.
	exe, _ := os.Executable()
	if _, err = s.AddBinding(Binding{ID: "synthetic", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: t.TempDir(), Model: "alias", Executable: exe, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err = s.PutPrice(id, 1, catalog.Price{Model: "alias", InputUSD: 1, OutputUSD: 2}); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.DeleteProviderModel("alias", m.Revision, "a", 2); err != nil {
		t.Fatal(err)
	}
	remaining, err := s.Model("alias")
	if err != nil || remaining.Revision != 2 || len(remaining.Candidates) != 1 || remaining.Candidates[0].ProviderID != "b" || !remaining.Tools || remaining.Strategy != "round-robin" {
		t.Fatal(remaining, err)
	}
	a, _ := s.Prices("a")
	b, _ := s.Prices("b")
	if len(a) != 0 || len(b) != 1 {
		t.Fatal(a, b)
	}
	if err = s.DeleteProviderModel("alias", 2, "b", 2); !errors.Is(err, ErrReferenced) {
		t.Fatal(err)
	}
	if _, err = s.Model("alias"); err != nil {
		t.Fatal("last candidate lost despite binding", err)
	}
}
