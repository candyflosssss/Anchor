package state

import (
	"errors"
	"testing"
)

func TestProviderProtocolEditUpdatesOnlyDefaultKey(t *testing.T) {
	s := newStore(t)
	secret := "synthetic-provider-protocol"
	p := provider("protocol-edit")
	p.Protocol = "anthropic"
	p, err := s.PutProvider(p, 0, &secret)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MutateProvider(p.ID, p.Revision, "key-put", "named", "synthetic-named", "", true, false, "responses"); err != nil {
		t.Fatal(err)
	}
	p, _ = s.Provider(p.ID)
	if err = s.PutKeyModelProtocol(p.ID, p.Revision, "default", "override-model", "anthropic"); err != nil {
		t.Fatal(err)
	}
	p, _ = s.Provider(p.ID)
	p.Protocol = "chat-completions"
	p, err = s.PutProvider(p, p.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := s.ProviderKeys(p.ID)
	if err != nil || len(keys) != 2 {
		t.Fatal(keys, err)
	}
	if keys[0].ID != "default" || keys[0].Protocol != "chat-completions" || keys[0].Secret != secret || keys[0].EffectiveProtocol("override-model") != "anthropic" || keys[1].Protocol != "responses" {
		t.Fatal("protocol isolation", keys)
	}
	// Omission preserves a default Key that was separately configured.
	if err = s.MutateProvider(p.ID, p.Revision, "key-put", "default", "", "", false, false, "responses"); err != nil {
		t.Fatal(err)
	}
	p, _ = s.Provider(p.ID)
	p.Protocol = ""
	p, err = s.PutProvider(p, p.Revision, nil)
	if err != nil || p.Protocol != "chat-completions" {
		t.Fatal(p, err)
	}
	keys, _ = s.ProviderKeys(p.ID)
	if keys[0].Protocol != "responses" || keys[0].Enabled {
		t.Fatal("omission changed Key", keys)
	}
	// Conflicts and invalid input leave both objects unchanged.
	stale := p
	stale.Protocol = "anthropic"
	if _, err = s.PutProvider(stale, p.Revision-1, nil); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	stale.Protocol = "unsupported"
	if _, err = s.PutProvider(stale, p.Revision, nil); err == nil {
		t.Fatal("invalid protocol accepted")
	}
	keys, _ = s.ProviderKeys(p.ID)
	if keys[0].Protocol != "responses" {
		t.Fatal("failed save changed Key", keys)
	}
	// Explicitly setting even the saved provider value updates default, without enabling it.
	p, err = s.PutProvider(p, p.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	keys, _ = s.ProviderKeys(p.ID)
	if keys[0].Protocol != "chat-completions" || keys[0].Enabled {
		t.Fatal("explicit protocol not applied", keys)
	}
}
