package state

import (
	"errors"
	"reflect"
	"testing"
)

func TestKeyProtocolsAndModelInheritance(t *testing.T) {
	s := newStore(t)
	secret := "synthetic-key"
	p := provider("p")
	p.Protocol = "anthropic"
	if _, err := s.PutProvider(p, 0, &secret); err != nil {
		t.Fatal(err)
	}
	mutate := func(action, target, protocol string, copyKeys bool) {
		t.Helper()
		p, err := s.Provider("p")
		if err != nil {
			t.Fatal(err)
		}
		if err = s.MutateProvider("p", p.Revision, action, target, "", "", true, copyKeys, protocol); err != nil {
			t.Fatal(err)
		}
	}
	override := func(model, protocol string) {
		t.Helper()
		p, _ := s.Provider("p")
		if err := s.PutKeyModelProtocol("p", p.Revision, "default", model, protocol); err != nil {
			t.Fatal(err)
		}
	}
	// A new key starts independently, while the initial credential retains its protocol.
	if err := s.MutateProvider("p", 1, "key-put", "second", "synthetic-second", "", true, false); err != nil {
		t.Fatal(err)
	}
	override("actual/model", "chat-completions")
	keys, _ := s.ProviderKeys("p")
	if keys[0].Protocol != "anthropic" || keys[0].EffectiveProtocol("actual/model") != "chat-completions" || keys[1].EffectiveProtocol("actual/model") != "responses" {
		t.Fatal(keys)
	}
	mutate("key-put", "default", "responses", false)
	keys, _ = s.ProviderKeys("p")
	if keys[0].EffectiveProtocol("other") != "responses" || keys[0].EffectiveProtocol("actual/model") != "chat-completions" {
		t.Fatal(keys)
	}
	override("actual/model", "inherit")
	mutate("key-put", "default", "anthropic", false)
	keys, _ = s.ProviderKeys("p")
	if keys[0].EffectiveProtocol("actual/model") != "anthropic" || len(keys[0].ModelProtocols) != 0 {
		t.Fatal(keys)
	}
	// Legacy provider edits and credential replacement never overwrite existing key settings.
	p, _ = s.Provider("p")
	p.Protocol = "chat-completions"
	if _, err := s.PutProvider(p, p.Revision, &secret); err != nil {
		t.Fatal(err)
	}
	keys, _ = s.ProviderKeys("p")
	if keys[0].Protocol != "anthropic" {
		t.Fatal(keys)
	}
	override("actual/model", "responses")
	before, _ := s.ProviderKeys("p")
	p, _ = s.Provider("p")
	for _, tc := range []struct {
		key, model, protocol string
		rev                  int64
		want                 error
	}{
		{"default", "actual/model", "invalid", p.Revision, ErrInvalid},
		{"missing", "actual/model", "responses", p.Revision, ErrNotFound},
		{"default", "bad\nmodel", "responses", p.Revision, ErrInvalid},
		{"default", "actual/model", "responses", p.Revision - 1, ErrConflict},
	} {
		if err := s.PutKeyModelProtocol("p", tc.rev, tc.key, tc.model, tc.protocol); !errors.Is(err, tc.want) {
			t.Fatal(err, tc.want)
		}
	}
	after, _ := s.ProviderKeys("p")
	current, _ := s.Provider("p")
	if !reflect.DeepEqual(before, after) || current.Revision != p.Revision {
		t.Fatal("failed writes changed configuration")
	}
	mutate("copy", "with-keys", "", true)
	copied, _ := s.ProviderKeys("with-keys")
	if !reflect.DeepEqual(before, copied) {
		t.Fatal(copied)
	}
	mutate("copy", "without-keys", "", false)
	copied, _ = s.ProviderKeys("without-keys")
	if len(copied) != 0 {
		t.Fatal(copied)
	}
	mutate("rename", "renamed", "", false)
	renamed, _ := s.ProviderKeys("renamed")
	if !reflect.DeepEqual(before, renamed) {
		t.Fatal(renamed)
	}
	p, _ = s.Provider("renamed")
	if err := s.MutateProvider(p.ID, p.Revision, "key-delete", "default", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM key_model_protocols WHERE provider_id='renamed'`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestV3MigratesEveryKeyProtocol(t *testing.T) {
	s := newStore(t)
	for _, protocol := range []string{"responses", "chat-completions", "anthropic"} {
		p := provider(protocol)
		p.Protocol = protocol
		secret := "synthetic"
		if _, err := s.PutProvider(p, 0, &secret); err != nil {
			t.Fatal(err)
		}
		if err := s.MutateProvider(p.ID, 1, "key-put", "other", secret, "", true, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`DROP TABLE key_model_protocols; ALTER TABLE provider_keys DROP COLUMN protocol; PRAGMA user_version=3;`); err != nil {
		t.Fatal(err)
	}
	dir := s.Dir
	s.Close()
	next, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	for _, protocol := range []string{"responses", "chat-completions", "anthropic"} {
		keys, err := next.ProviderKeys(protocol)
		if err != nil || len(keys) != 2 {
			t.Fatal(keys, err)
		}
		for _, k := range keys {
			if k.Protocol != protocol || k.EffectiveProtocol("actual") != protocol || len(k.ModelProtocols) != 0 {
				t.Fatal(k)
			}
		}
	}
}
