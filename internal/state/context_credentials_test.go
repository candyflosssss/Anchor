package state

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestContextCredentialIsolationAndLegacy(t *testing.T) {
	s := newStore(t)
	local := Context{Name: "local", Endpoint: "http://127.0.0.1:17861", InstanceID: "legacy-instance", CredentialRef: "local-owner"}
	if err := s.PutContext(local); err != nil {
		t.Fatal(err)
	}
	remote := Context{Name: "server", Transport: "ssh", InstanceID: "remote-instance", CredentialRef: "ssh-manage", SSHTarget: "user@server", RemoteExecutable: "anchor", RemoteControlPort: 17861, AccessID: "remote-access"}
	if err := s.PutContextWithCredential(remote, "synthetic-management-secret"); err != nil {
		t.Fatal(err)
	}
	secret, err := s.ContextCredential("server")
	if err != nil || secret != "synthetic-management-secret" {
		t.Fatal(err)
	}
	rows, err := s.Contexts()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(rows)
	if strings.Contains(string(raw), secret) || len(rows) != 2 {
		t.Fatal("context list leaked credentials")
	}
	if got, err := s.Context("local"); err != nil || got.Transport != "" || got.Validate() != nil {
		t.Fatal("legacy context changed", err)
	}
	if _, err := s.ContextCredential("local"); err != ErrNotFound {
		t.Fatal("local unexpectedly has remote credential")
	}
	remote.InstanceID = "replacement"
	if err := s.PutContextWithCredential(remote, "replacement-secret"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Context("server")
	if err != nil || got.InstanceID != "replacement" {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DELETE FROM contexts WHERE name='server'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ContextCredential("server"); err != ErrNotFound {
		t.Fatal("orphaned credential")
	}
}
func TestSSHContextPolicies(t *testing.T) {
	good := Context{Name: "server", Transport: "ssh", InstanceID: "remote", CredentialRef: "ssh-manage", SSHTarget: "user@host", RemoteExecutable: "/home/user/.local/bin/anchor", RemoteControlPort: 17861, AccessID: "access"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Context){
		func(c *Context) { c.SSHTarget = "-oProxyCommand=anything" }, func(c *Context) { c.SSHTarget = "host\ncommand" }, func(c *Context) { c.SSHTarget = "host command" },
		func(c *Context) { c.RemoteExecutable = "anchor; id" }, func(c *Context) { c.RemoteExecutable = "$(id)" }, func(c *Context) { c.RemoteExecutable = "-anchor" },
		func(c *Context) { c.RemoteControlPort = 65536 }, func(c *Context) { c.SSHPort = -1 }, func(c *Context) { c.Endpoint = "http://127.0.0.1:1234" },
	} {
		c := good
		mutate(&c)
		if c.Validate() == nil {
			t.Fatal("unsafe context accepted", c)
		}
	}
}
