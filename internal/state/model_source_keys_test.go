package state

import (
	"errors"
	"testing"
)

func TestModelSourceKeysPersistenceAndValidation(t *testing.T) {
	s := newStore(t)
	secret := "synthetic"
	p, err := s.PutProvider(provider("p"), 0, &secret)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.PutModel(Model{ID: "alias", Strategy: "cache-priority", Candidates: []Candidate{{ProviderID: p.ID, Model: "up", KeyIDs: []string{"default"}}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	bad := m
	bad.Candidates = []Candidate{{ProviderID: p.ID, Model: "up", KeyIDs: []string{"missing"}}}
	if _, err = s.PutModel(bad, m.Revision); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	bad.Candidates[0].KeyIDs = []string{"default", "default"}
	if bad.Validate() == nil {
		t.Fatal("duplicate key IDs accepted")
	}
	if _, err = s.db.Exec(`PRAGMA user_version=5`); err != nil {
		t.Fatal(err)
	}
	migrated, err := Open(s.Dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	got, err := migrated.Model("alias")
	if err != nil || len(got.Candidates[0].KeyIDs) != 1 || got.Strategy != "cache-priority" {
		t.Fatal(got, err)
	}
	var version int
	migrated.db.QueryRow(`PRAGMA user_version`).Scan(&version)
	if version != 9 {
		t.Fatal(version)
	}
	m.Candidates[0].KeyIDs = nil
	m, err = s.PutModel(m, m.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MutateProvider("p", 1, "key-delete", "default", "", "", false, false); err != nil {
		t.Fatal("unrestricted model deletion failed", err)
	}
}

func TestAnonymousSourceKeyDistinguishesSavedKeyFromAnonymousFallback(t *testing.T) {
	s := newStore(t)
	p, err := s.PutProvider(provider("p"), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{ID: "alias", Candidates: []Candidate{{ProviderID: p.ID, Model: "up", KeyIDs: []string{"anonymous"}}}}
	if _, err = s.PutModel(m, 0); err != nil {
		t.Fatal(err)
	}
	if err = s.MutateProvider(p.ID, p.Revision, "key-put", "other", "synthetic", "responses", true, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PutModel(m, 1); !errors.Is(err, ErrInvalid) {
		t.Fatal("anonymous fallback accepted with real Keys", err)
	}
	if err = s.MutateProvider(p.ID, p.Revision+1, "key-put", "anonymous", "synthetic", "responses", true, false); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PutModel(m, 1); err != nil {
		t.Fatal("saved Key named anonymous must be valid", err)
	}
}
