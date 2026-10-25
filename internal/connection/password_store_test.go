package connection

import (
	"errors"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/zalando/go-keyring"
)

type testPasswordVault struct {
	values                       map[string]string
	gets, sets, deletes          int
	failGet, failSet, failDelete bool
	ignoreSet, ignoreDelete      bool
}

func (v *testPasswordVault) Get(service, account string) (string, error) {
	v.gets++
	if v.failGet {
		return "", errors.New("private vault diagnostic")
	}
	secret, ok := v.values[service+account]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return secret, nil
}
func (v *testPasswordVault) Set(service, account, secret string) error {
	v.sets++
	if v.failSet {
		return errors.New("private vault diagnostic")
	}
	if v.ignoreSet {
		return nil
	}
	if v.values == nil {
		v.values = map[string]string{}
	}
	v.values[service+account] = secret
	return nil
}
func (v *testPasswordVault) Delete(service, account string) error {
	v.deletes++
	if v.failDelete {
		return errors.New("private vault diagnostic")
	}
	if v.ignoreDelete {
		return nil
	}
	delete(v.values, service+account)
	return nil
}
func passwordStoreFixture(t *testing.T) (*state.Store, state.Context) {
	t.Helper()
	s, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.Initialize("127.0.0.1:17861", "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	c := state.Context{Name: "saved-device", Transport: "ssh", SSHTarget: "fixture@192.0.2.10", SSHAuth: "password", SSHHostKey: "fixture-host-key", InstanceID: "fixture-instance", AccessID: "fixture-access", CredentialRef: "ssh-manage", RemoteExecutable: "anchor", RemoteControlPort: 17861}
	if err := s.PutContextWithCredential(c, "fixture-token"); err != nil {
		t.Fatal(err)
	}
	return s, c
}
func TestPasswordVaultScopeAndMetadata(t *testing.T) {
	s, c := passwordStoreFixture(t)
	vault := &testPasswordVault{}
	p := Passwords{Vault: vault}
	if saved, err := p.Saved(s, c); err != nil || saved || vault.gets != 0 {
		t.Fatal("default saved password", saved, err)
	}
	if err := p.Save(s, c, "fixture-only-secret"); err != nil {
		t.Fatal(err)
	}
	reads := vault.gets
	if saved, err := p.Saved(s, c); err != nil || !saved || vault.gets != reads {
		t.Fatal("metadata accessed vault", saved, err)
	}
	ref, err := s.ContextPasswordRef(c.Name)
	if err != nil || len(ref) != 64 || strings.Contains(ref, "secret") {
		t.Fatal("unsafe metadata", err)
	}
	for _, change := range []func(*state.Context){func(c *state.Context) { c.Name = "other-device" }, func(c *state.Context) { c.SSHTarget = "other@192.0.2.10" }, func(c *state.Context) { c.SSHPort = 2222 }, func(c *state.Context) { c.SSHHostKey = "other-host" }, func(c *state.Context) { c.InstanceID = "other-instance" }} {
		other := c
		change(&other)
		key, err := passwordRef(s, other)
		if err != nil || key == ref {
			t.Fatal("credential target not isolated", other, err)
		}
	}
	otherStore, _ := passwordStoreFixture(t)
	if key, _ := passwordRef(otherStore, c); key == ref {
		t.Fatal("data directories share credentials")
	}
	port22 := c
	port22.SSHPort = 22
	if key, _ := passwordRef(s, port22); key != ref {
		t.Fatal("default SSH port not normalized")
	}
	if err := p.Forget(s, c); err != nil {
		t.Fatal(err)
	}
	if saved, _ := p.Saved(s, c); saved || len(vault.values) != 0 {
		t.Fatal("password not forgotten")
	}
}
func TestPasswordVaultFailuresNeverFallBackOrExposeDetails(t *testing.T) {
	s, c := passwordStoreFixture(t)
	vault := &testPasswordVault{failSet: true}
	p := Passwords{Vault: vault}
	if err := p.Save(s, c, "fixture-only-secret"); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("unsafe save failure", err)
	}
	if saved, _ := p.Saved(s, c); saved {
		t.Fatal("failed save marked remembered")
	}
	vault.failSet = false
	if err := p.Save(s, c, "fixture-only-secret"); err != nil {
		t.Fatal(err)
	}
	vault.failGet = true
	if _, err := p.Open(t.Context(), s, c, ""); err == nil || !strings.Contains(err.Error(), "credential_store") || strings.Contains(err.Error(), "private") {
		t.Fatal("unsafe read failure", err)
	}
	vault.failDelete = true
	if err := p.Forget(s, c); err == nil {
		t.Fatal("failed forget reported success")
	}
	if saved, _ := p.Saved(s, c); !saved {
		t.Fatal("failed forget cleared metadata")
	}
	vault.failDelete = false
	vault.failGet = false
	if err := p.Forget(s, c); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordVaultVerifiesDismissedSystemPrompts(t *testing.T) {
	s, c := passwordStoreFixture(t)
	vault := &testPasswordVault{ignoreSet: true}
	p := Passwords{Vault: vault}
	if err := p.Save(s, c, "fixture-only-secret"); err == nil {
		t.Fatal("dismissed save reported success")
	}
	if saved, _ := p.Saved(s, c); saved {
		t.Fatal("dismissed save set marker")
	}
	vault.ignoreSet = false
	if err := p.Save(s, c, "fixture-only-secret"); err != nil {
		t.Fatal(err)
	}
	vault.ignoreDelete = true
	if err := p.Forget(s, c); err == nil {
		t.Fatal("dismissed delete reported success")
	}
	if saved, _ := p.Saved(s, c); !saved {
		t.Fatal("dismissed delete cleared marker")
	}
	vault.ignoreDelete = false
	if err := p.Forget(s, c); err != nil {
		t.Fatal(err)
	}
}
