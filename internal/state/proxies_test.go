package state

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func proxyMutation(t *testing.T, s *Store, in ProxyAction) {
	t.Helper()
	w, e := s.ProxyWorkspace()
	if e != nil {
		t.Fatal(e)
	}
	in.Approved = true
	in.ExpectedRevision = w.Revision
	if e = s.MutateProxy(in, func(_, raw, _ string) (string, error) { return raw, nil }); e != nil {
		t.Fatal(e)
	}
}
func TestProxyInheritanceSecretsReferencesAndCopy(t *testing.T) {
	s := newStore(t)
	secret := "synthetic-key"
	if _, e := s.PutProvider(provider("p"), 0, &secret); e != nil {
		t.Fatal(e)
	}
	hidden := "http://user:synthetic-password@127.0.0.1:18080"
	proxyMutation(t, s, ProxyAction{Action: "put", Profile: ProxyProfile{ID: "edge", Name: "Edge", Kind: "http"}, Secret: &hidden})
	proxyMutation(t, s, ProxyAction{Action: "bind", Binding: ProxyBinding{ProviderID: "p", Mode: "proxy", ProxyID: "edge"}})
	route, e := s.ProxySecretFor("p", "default")
	if e != nil || route == nil || route.Secret != hidden {
		t.Fatal(route, e)
	}
	proxyMutation(t, s, ProxyAction{Action: "bind", Binding: ProxyBinding{ProviderID: "p", KeyID: "default", Mode: "direct"}})
	route, e = s.ProxySecretFor("p", "default")
	if e != nil || route != nil {
		t.Fatal("direct override", route, e)
	}
	proxyMutation(t, s, ProxyAction{Action: "bind", Binding: ProxyBinding{ProviderID: "p", KeyID: "default", Mode: "inherit"}})
	proxyMutation(t, s, ProxyAction{Action: "put", Profile: ProxyProfile{ID: "edge", Name: "Renamed", Kind: "http"}})
	route, e = s.ProxySecretFor("p", "default")
	if e != nil || route == nil || route.Secret != hidden {
		t.Fatal("preserved secret", route, e)
	}
	w, _ := s.ProxyWorkspace()
	public, _ := json.Marshal(w)
	if strings.Contains(string(public), "synthetic") || strings.Contains(string(public), "18080") {
		t.Fatal("secret leaked")
	}
	if e = s.MutateProxy(ProxyAction{Action: "delete", Profile: ProxyProfile{ID: "edge"}, Approved: true, ExpectedRevision: w.Revision}, nil); !errors.Is(e, ErrReferenced) {
		t.Fatal(e)
	}
	if e = s.MutateProxy(ProxyAction{Action: "delete", Profile: ProxyProfile{ID: "edge"}, Approved: true, ExpectedRevision: 1}, nil); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if e = s.MutateProvider("p", 1, "copy", "copy", "", "", false, true); e != nil {
		t.Fatal(e)
	}
	if route, e = s.ProxySecretFor("copy", "default"); e != nil || route == nil {
		t.Fatal("copy", route, e)
	}
	p, _ := s.Provider("p")
	if e = s.MutateProvider("p", p.Revision, "rename", "renamed", "", "", false, false); e != nil {
		t.Fatal(e)
	}
	if route, e = s.ProxySecretFor("renamed", "default"); e != nil || route == nil {
		t.Fatal("cascade", route, e)
	}
	if _, e = s.ProxySecretFor("renamed", "deleted-key"); !errors.Is(e, ErrNotFound) {
		t.Fatal("missing key must fail closed", e)
	}
}
func TestProxyMutationValidation(t *testing.T) {
	s := newStore(t)
	for _, in := range []ProxyAction{{Action: "put", ExpectedRevision: 1}, {Action: "bind", Approved: true, ExpectedRevision: 1, Binding: ProxyBinding{ProviderID: "missing", Mode: "direct"}}, {Action: "bogus", Approved: true, ExpectedRevision: 1}} {
		if e := s.MutateProxy(in, func(_, raw, _ string) (string, error) { return raw, nil }); e == nil {
			t.Fatal("invalid mutation accepted")
		}
	}
}

func TestSystemProxyInheritanceValidationAndPersistence(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { s.Close() }()
	secret := "synthetic"
	if _, e = s.PutProvider(provider("p"), 0, &secret); e != nil {
		t.Fatal(e)
	}
	proxyMutation(t, s, ProxyAction{Action: "bind", Binding: ProxyBinding{ProviderID: "p", Mode: "system"}})
	r, e := s.ProxySecretFor("p", "default")
	if e != nil || r == nil || r.Mode != "system" {
		t.Fatal(r, e)
	}
	proxyMutation(t, s, ProxyAction{Action: "bind", Binding: ProxyBinding{ProviderID: "p", KeyID: "default", Mode: "direct"}})
	if r, e = s.ProxySecretFor("p", "default"); e != nil || r != nil {
		t.Fatal(r, e)
	}
	proxyMutation(t, s, ProxyAction{Action: "bind", Binding: ProxyBinding{ProviderID: "p", KeyID: "default", Mode: "system"}})
	w, _ := s.ProxyWorkspace()
	if e = s.MutateProxy(ProxyAction{Action: "bind", Approved: true, ExpectedRevision: w.Revision, Binding: ProxyBinding{ProviderID: "p", Mode: "system", ProxyID: "unexpected"}}, nil); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if e = s.MutateProxy(ProxyAction{Action: "bind", Approved: true, ExpectedRevision: 1, Binding: ProxyBinding{ProviderID: "p", Mode: "system"}}, nil); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(dir, false)
	if e != nil {
		t.Fatal(e)
	}
	if r, e = s.ProxySecretFor("p", "default"); e != nil || r == nil || r.Mode != "system" {
		t.Fatal(r, e)
	}
}

func TestProxyListenPortMigrationValidationAndRetention(t *testing.T) {
	s := newStore(t)
	raw := "synthetic-node"
	proxyMutation(t, s, ProxyAction{Action: "put", Profile: ProxyProfile{ID: "edge", Name: "Edge", Kind: "mihomo"}, Secret: &raw})
	// Emulate a schema9 instance with an existing node.
	if _, e := s.db.Exec(`ALTER TABLE proxy_profiles DROP COLUMN listen_port; PRAGMA user_version=9;`); e != nil {
		t.Fatal(e)
	}
	dir := s.Dir
	s.Close()
	var e error
	s, e = Open(dir, false)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { s.Close() }()
	w, e := s.ProxyWorkspace()
	if e != nil || w.Profiles[0].ListenPort == nil || *w.Profiles[0].ListenPort != 0 {
		t.Fatal(w, e)
	}
	port := 17890
	proxyMutation(t, s, ProxyAction{Action: "put", Profile: ProxyProfile{ID: "edge", Name: "Edge", Kind: "mihomo", ListenPort: &port}})
	proxyMutation(t, s, ProxyAction{Action: "put", Profile: ProxyProfile{ID: "edge", Name: "Renamed", Kind: "mihomo"}})
	w, _ = s.ProxyWorkspace()
	if *w.Profiles[0].ListenPort != port {
		t.Fatal("omitted port did not retain fixed port")
	}
	for _, tc := range []struct {
		id, kind string
		port     int
	}{{"edge", "mihomo", -1}, {"edge", "mihomo", 65536}, {"other", "mihomo", port}, {"edge", "http", port}} {
		action := ProxyAction{Action: "put", Approved: true, ExpectedRevision: w.Revision, Profile: ProxyProfile{ID: tc.id, Name: "Test", Kind: tc.kind, ListenPort: &tc.port}, Secret: &raw}
		if e = s.MutateProxy(action, func(_, raw, _ string) (string, error) { return raw, nil }); !errors.Is(e, ErrInvalid) {
			t.Fatal(tc, e)
		}
		next, _ := s.ProxyWorkspace()
		if next.Revision != w.Revision {
			t.Fatal("invalid port changed revision")
		}
	}
	port = 0
	proxyMutation(t, s, ProxyAction{Action: "put", Profile: ProxyProfile{ID: "edge", Name: "Edge", Kind: "mihomo", ListenPort: &port}})
	s.Close()
	s, e = Open(dir, false)
	if e != nil {
		t.Fatal(e)
	}
	w, _ = s.ProxyWorkspace()
	if *w.Profiles[0].ListenPort != 0 {
		t.Fatal("automatic port was not persisted")
	}
}
