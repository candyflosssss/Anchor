package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestProviderInitialKeysAtomicCreation(t *testing.T) {
	s := newStore(t)
	p := provider("multi-key")
	p.BaseURLs = []string{p.BaseURL, "https://192.0.2.10:8443/v1"}
	secret := "synthetic-default"
	keys := []InitialProviderKey{{ID: "backup", Protocol: "anthropic", Secret: "synthetic-backup"}, {ID: "备用", Protocol: "chat-completions", Secret: "synthetic-secondary"}}
	saved, err := s.PutProviderWithKeys(p, 0, &secret, keys)
	if err != nil || saved.Revision != 1 || !saved.HasSecret || len(saved.BaseURLs) != 2 {
		t.Fatal("creation failed", err)
	}
	stored, err := s.ProviderKeys(p.ID)
	if err != nil || len(stored) != 3 || stored[0].ID != "backup" || stored[0].Protocol != "anthropic" || stored[0].Secret != keys[0].Secret || stored[1].ID != "default" || stored[1].Secret != secret {
		t.Fatal("Keys not saved together")
	}
	raw, _ := json.Marshal(struct {
		Provider Provider
		Keys     []ProviderKey
	}{saved, stored})
	if strings.Contains(string(raw), "synthetic-") {
		t.Fatal("credentials leaked")
	}
	if _, err = s.PutProviderWithKeys(p, 0, &secret, keys); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err = s.PutProviderWithKeys(p, 1, &secret, keys); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	// Editing all request roots preserves every Key and increments revision only once.
	saved.BaseURL = "https://changed.example.invalid/v1"
	saved.BaseURLs = []string{saved.BaseURL, "https://192.0.2.11/v1"}
	updated, err := s.PutProvider(saved, saved.Revision, nil)
	if err != nil || updated.Revision != 2 {
		t.Fatal(err)
	}
	after, _ := s.ProviderKeys(p.ID)
	if len(after) != 3 || after[0].Secret != keys[0].Secret || after[1].Secret != secret || after[2].Protocol != "chat-completions" {
		t.Fatal("address edit changed Keys")
	}
}

func TestProviderInitialKeysRejectWithoutPartialWrite(t *testing.T) {
	valid := InitialProviderKey{ID: "backup", Protocol: "responses", Secret: "synthetic-key"}
	for name, key := range map[string]InitialProviderKey{
		"name":      {ID: "bad name", Protocol: "responses", Secret: "synthetic-key"},
		"reserved":  {ID: "default", Protocol: "responses", Secret: "synthetic-key"},
		"protocol":  {ID: "extra", Protocol: "invalid", Secret: "synthetic-key"},
		"blank":     {ID: "extra", Protocol: "responses", Secret: " "},
		"newline":   {ID: "extra", Protocol: "responses", Secret: "bad\nkey"},
		"large":     {ID: "extra", Protocol: "responses", Secret: strings.Repeat("密", 2731)},
		"duplicate": valid,
	} {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			secret := "synthetic-default"
			_, err := s.PutProviderWithKeys(provider("invalid"), 0, &secret, []InitialProviderKey{valid, key})
			if !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid Key accepted", err)
			}
			rows, _ := s.Providers()
			if len(rows) != 0 {
				t.Fatal("partial provider created")
			}
		})
	}
}

func TestProviderInitialKeysLimitAndRollback(t *testing.T) {
	s := newStore(t)
	keys := make([]InitialProviderKey, 64)
	for i := range keys {
		keys[i] = InitialProviderKey{ID: fmt.Sprintf("key-%d", i), Protocol: "responses", Secret: "synthetic-key"}
	}
	secret := "synthetic-default"
	if _, err := s.PutProviderWithKeys(provider("limit"), 0, &secret, keys); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := s.PutProviderWithKeys(provider("limit"), 0, &secret, keys[:63]); err != nil {
		t.Fatal(err)
	}
	// A late storage failure must roll back the provider, default Key and earlier Keys.
	_, err := s.db.Exec(`CREATE TRIGGER reject_initial_key BEFORE INSERT ON provider_keys WHEN NEW.id='key-1' BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.PutProviderWithKeys(provider("rollback"), 0, &secret, keys[:2]); err == nil {
		t.Fatal("expected write failure")
	}
	if _, err = s.Provider("rollback"); !errors.Is(err, ErrNotFound) {
		t.Fatal("partial provider after SQL failure", err)
	}
	stored, _ := s.ProviderKeys("rollback")
	if len(stored) != 0 {
		t.Fatal("partial Keys after SQL failure")
	}
}

func TestProviderInitialKeysWithoutDefault(t *testing.T) {
	s := newStore(t)
	p, err := s.PutProviderWithKeys(provider("named-only"), 0, nil, []InitialProviderKey{{ID: "named", Protocol: "anthropic", Secret: "synthetic-named"}})
	if err != nil || !p.HasSecret {
		t.Fatal("creation missing Key status", err)
	}
	p, err = s.Provider(p.ID)
	if err != nil || !p.HasSecret || p.Secret != "" {
		t.Fatal("read conflates named Key with default credential")
	}
	rows, err := s.Providers()
	if err != nil || len(rows) != 1 || !rows[0].HasSecret {
		t.Fatal("list missing named Key status", err)
	}
}
