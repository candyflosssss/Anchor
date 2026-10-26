package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBindingCredentialsAndRevisions(t *testing.T) {
	st := newStore(t)
	exe, _ := os.Executable()
	b := Binding{ID: "codex-test", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: t.TempDir(), Model: "public", Executable: exe, Enabled: true}
	saved, e := st.AddBinding(b)
	if e != nil || saved.Revision != 1 {
		t.Fatal(saved, e)
	}
	token, e := st.BindingToken(b.ID)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(saved)
	if strings.Contains(string(raw), token) {
		t.Fatal("secret leaked")
	}
	if st.Authenticate(token, "manage") {
		t.Fatal("binding token authorizes management")
	}
	if found, e := st.BindingForToken(token); e != nil || found.ID != b.ID {
		t.Fatal(found, e)
	}
	b.ID = "overlap"
	if _, e = st.AddBinding(b); !errors.Is(e, ErrConflict) {
		t.Fatal("overlap accepted", e)
	}
	if _, e = st.EnableBinding(saved.ID, 0, false); !errors.Is(e, ErrConflict) {
		t.Fatal("stale revision accepted")
	}
	disabled, e := st.EnableBinding(saved.ID, 1, false)
	if e != nil || disabled.Revision != 2 {
		t.Fatal(disabled, e)
	}
	if _, e = st.BindingForToken(token); !errors.Is(e, ErrNotFound) {
		t.Fatal("disabled token accepted", e)
	}
	if _, e = st.BindingToken(saved.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("disabled token exported", e)
	}
}
func TestV1MigrationPreservesIdentityAndConfig(t *testing.T) {
	s := newStore(t)
	dir := s.Dir
	before, e := s.Settings()
	if e != nil {
		t.Fatal(e)
	}
	token, _ := s.OwnerToken()
	if _, e = s.db.Exec(`DROP TABLE provider_platforms; DROP TABLE bindings; DROP TABLE key_model_protocols; DROP TABLE provider_keys; DROP TABLE provider_strategy; DROP TABLE provider_prices; DROP TABLE usage_totals; ALTER TABLE proxy_profiles DROP COLUMN listen_port; PRAGMA user_version=1;`); e != nil {
		t.Fatal(e)
	}
	s.Close()
	migrated, e := Open(dir, false)
	if e != nil {
		t.Fatal(e)
	}
	defer migrated.Close()
	after, _ := migrated.Settings()
	if before != after {
		t.Fatal("identity migrated incorrectly")
	}
	if now, _ := migrated.OwnerToken(); now != token {
		t.Fatal("owner token rotated")
	}
	if bs, e := migrated.Bindings(); e != nil || len(bs) != 0 {
		t.Fatal(bs, e)
	}
	if _, e = os.Stat(filepath.Join(dir, "state.db")); e != nil {
		t.Fatal(e)
	}
}

func TestAllCompatibleModelsAndExplicitMigration(t *testing.T) {
	st := newStore(t)
	for _, p := range []Provider{
		{ID: "on", BaseURL: "http://127.0.0.1:1234/v1", Protocol: "responses", Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 1},
		{ID: "off", BaseURL: "http://127.0.0.1:1234/v1", Protocol: "responses", Enabled: false, TimeoutSeconds: 10, MaxConcurrent: 1},
	} {
		if _, err := st.PutProvider(p, 0, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		id, provider, version   string
		tools, stream, metadata bool
	}{
		{"default", "on", "0.159.3", true, true, true}, {"alternative", "on", "0.159.3", true, true, true},
		{"disabled", "off", "0.159.3", true, true, true}, {"wrong-version", "on", "0.158.0", true, true, true},
		{"no-tools", "on", "0.159.3", false, true, true}, {"no-stream", "on", "0.159.3", true, false, true},
		{"no-metadata", "on", "0.159.3", true, true, false},
	} {
		m := Model{ID: tc.id, Tools: tc.tools, Streaming: tc.stream, Candidates: []Candidate{{ProviderID: tc.provider, Model: "actual"}}}
		if tc.metadata {
			m.Codex = fixtureMetadata(t, tc.version)
		}
		if _, err := st.PutModel(m, 0); err != nil {
			t.Fatal(err)
		}
	}
	exe, _ := os.Executable()
	legacy, err := st.AddBinding(Binding{ID: "legacy", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: t.TempDir(), Executable: exe, Enabled: true, Model: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if allowed, _ := st.BindingAllows(legacy, "alternative"); allowed {
		t.Fatal("silent broadening of legacy binding")
	}
	token, _ := st.BindingToken(legacy.ID)
	b, err := st.SetBindingModels(legacy.ID, 1, "default", "all-compatible", "0.159.3")
	if err != nil || b.Revision != 2 {
		t.Fatal(b, err)
	}
	models, err := st.BindingModels(b)
	if err != nil || len(models) != 2 {
		t.Fatal(models, err)
	}
	for _, id := range []string{"default", "alternative", "disabled", "wrong-version", "no-tools", "no-stream", "no-metadata", "unknown"} {
		allowed, err := st.BindingAllows(b, id)
		if err != nil || allowed != (id == "default" || id == "alternative") {
			t.Fatal(id, allowed, err)
		}
	}
	if _, err = st.SetBindingModels(b.ID, 1, "alternative", "single", ""); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if now, _ := st.BindingToken(b.ID); now != token {
		t.Fatal("policy update rotated credential")
	}
	p, _ := st.Provider("on")
	p.Enabled = false
	if _, err = st.PutProvider(p, p.Revision, nil); err != nil {
		t.Fatal(err)
	}
	if models, err = st.BindingModels(b); err != nil || len(models) != 0 {
		t.Fatal("disabled provider still eligible", models, err)
	}
}

func TestDSHProfileOwnership(t *testing.T) {
	for _, first := range []string{"dsh", "dsh-desktop"} {
		t.Run(first, func(t *testing.T) {
			s := newStore(t)
			root := t.TempDir()
			exe, _ := os.Executable()
			makeBinding := func(id, adapter string) Binding {
				return Binding{ID: id, AdapterID: adapter, AdapterVersion: "1", ConfigRoot: root, Model: "public", Executable: exe, Enabled: true}
			}
			second := "dsh"
			if first == "dsh" {
				second = "dsh-desktop"
			}
			for _, adapter := range []string{first, second} {
				b, err := s.AddBinding(makeBinding(adapter, adapter))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.EnableBinding(b.ID, b.Revision, false); err != nil {
					t.Fatal(err)
				}
			}
			for _, adapter := range []string{"dsh", "dsh-desktop", "codex", "claude"} {
				if _, err := s.AddBinding(makeBinding("duplicate-"+adapter, adapter)); !errors.Is(err, ErrConflict) {
					t.Fatal("overlapping ownership accepted", adapter, err)
				}
			}
			// Existing Web ownership keys are unchanged, and survive reopening.
			var key string
			if err := s.db.QueryRow(`SELECT root FROM bindings WHERE id='dsh'`).Scan(&key); err != nil || key != canonicalBindingRoot(root) {
				t.Fatal(key, err)
			}
			dir := s.Dir
			s.Close()
			reopened, err := Open(dir, false)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			bindings, err := reopened.Bindings()
			if err != nil || len(bindings) != 2 {
				t.Fatal(bindings, err)
			}
		})
	}
	// A Desktop binding must not bypass another adapter's whole-root ownership.
	s := newStore(t)
	exe, _ := os.Executable()
	b := Binding{ID: "other", AdapterID: "claude", AdapterVersion: "1", ConfigRoot: t.TempDir(), Model: "public", Executable: exe}
	if _, err := s.AddBinding(b); err != nil {
		t.Fatal(err)
	}
	b.ID, b.AdapterID = "desktop", "dsh-desktop"
	if _, err := s.AddBinding(b); !errors.Is(err, ErrConflict) {
		t.Fatal("Desktop bypassed existing ownership", err)
	}
}
