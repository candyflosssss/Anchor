package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/zalando/go-keyring"
)

type isolatedPasswordVault struct {
	password               string
	fail                   bool
	reads, writes, deletes int
}

func (v *isolatedPasswordVault) Get(_, _ string) (string, error) {
	v.reads++
	if v.fail {
		return "", errors.New("private vault detail")
	}
	if v.password == "" {
		return "", keyring.ErrNotFound
	}
	return v.password, nil
}
func (v *isolatedPasswordVault) Set(_, _, secret string) error {
	v.writes++
	if v.fail {
		return errors.New("private vault detail")
	}
	v.password = secret
	return nil
}
func (v *isolatedPasswordVault) Delete(_, _ string) error {
	v.deletes++
	if v.fail {
		return errors.New("private vault detail")
	}
	v.password = ""
	return nil
}

func TestRememberedSSHPasswordBridgeAndCLI(t *testing.T) {
	input, _ := isolatedSSH(t)
	input.Password, input.RememberPassword = "fixture-password", true
	local, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	if _, err := local.Initialize("127.0.0.1:17861", "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	vault := &isolatedPasswordVault{}
	options := Options{Dir: local.Dir, PasswordVault: vault}
	s := NewSession(options)
	defer s.Close()
	b := NewBridge(s)
	r := b.ConnectSSH(input)
	if r.Problem != nil || r.Warning != nil || r.Data == nil || vault.password != input.Password {
		t.Fatalf("remember: %+v", r)
	}
	profiles := b.Connections()
	if profiles.Problem != nil || profiles.Data[1].PasswordSaved == nil || !*profiles.Data[1].PasswordSaved || vault.reads != 1 {
		t.Fatalf("password metadata: %+v", profiles)
	}
	s.Close()
	fresh := NewSession(options)
	defer fresh.Close()
	fb := NewBridge(fresh)
	if r := fb.Start(); r.Problem != nil || r.Data == nil || vault.reads != 2 {
		t.Fatalf("automatic saved login: %+v", r)
	}
	var out, stderr bytes.Buffer
	app := cli.App{Out: &out, Err: &stderr, PasswordVault: vault}
	for _, args := range [][]string{{"context", "password", "status", input.Name}, {"context", "test", input.Name}, {"--context", input.Name, "status"}} {
		if err := app.Run(t.Context(), append([]string{"--data-dir", local.Dir}, args...)); err != nil {
			t.Fatal(args, err)
		}
	}
	if !strings.Contains(out.String(), `"password_saved":true`) || strings.Contains(out.String()+stderr.String(), input.Password) {
		t.Fatal("CLI metadata/secret disclosure")
	}
	app.In = strings.NewReader(input.Password)
	if err := app.Run(t.Context(), []string{"--data-dir", local.Dir, "--ssh-password-stdin", "context", "password", "save", input.Name}); err != nil {
		t.Fatal("CLI verified save", err)
	}
	if r := fb.ForgetConnectionPassword(SelectConnection{Name: input.Name}); r.Problem != nil || *r.Data[1].PasswordSaved || vault.password != "" {
		t.Fatalf("forget: %+v", r)
	}
	if r := fb.Status(); r.Problem != nil {
		t.Fatal("forget disconnected active session", r.Problem)
	}
	if err := app.Run(t.Context(), []string{"--data-dir", local.Dir, "context", "password", "forget", input.Name}); err != nil {
		t.Fatal("idempotent CLI forget", err)
	}
	fresh.Close()
	last := NewSession(options)
	defer last.Close()
	if r := NewBridge(last).Start(); r.Problem == nil || r.Problem.Code != "ssh_auth" {
		t.Fatalf("forgotten password reused: %+v", r)
	}
	for _, name := range []string{"state.db", "state.db-wal"} {
		raw, err := os.ReadFile(filepath.Join(local.Dir, name))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte(input.Password)) {
			t.Fatal("SSH password persisted to database", name)
		}
	}
	encoded, _ := json.Marshal(profiles)
	if bytes.Contains(encoded, []byte(input.Password)) {
		t.Fatal("password returned to frontend")
	}
}

func TestSSHPasswordSaveFailureKeepsReadyConnection(t *testing.T) {
	input, _ := isolatedSSH(t)
	input.Password, input.RememberPassword = "fixture-password", true
	local, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	_, _ = local.Initialize("127.0.0.1:17861", "127.0.0.1:17860")
	vault := &isolatedPasswordVault{fail: true}
	s := NewSession(Options{Dir: local.Dir, PasswordVault: vault})
	defer s.Close()
	b := NewBridge(s)
	r := b.ConnectSSH(input)
	if r.Problem != nil || r.Data == nil || r.Warning == nil || r.Warning.Code != "credential_store" {
		t.Fatalf("partial save outcome: %+v", r)
	}
	if r := b.Status(); r.Problem != nil {
		t.Fatal("save failure lost connection", r.Problem)
	}
	if r := b.Connections(); r.Problem != nil || *r.Data[1].PasswordSaved {
		t.Fatalf("failed save marked saved: %+v", r)
	}
}

func TestRememberedSSHPasswordRecoveryAndExplicitSave(t *testing.T) {
	input, _ := isolatedSSH(t)
	input.Password, input.RememberPassword = "wrong-password", true
	local, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	if _, err := local.Initialize("127.0.0.1:17861", "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	vault := &isolatedPasswordVault{}
	options := Options{Dir: local.Dir, PasswordVault: vault}
	s := NewSession(options)
	defer s.Close()
	b := NewBridge(s)
	if r := b.ConnectSSH(input); r.Problem == nil || r.Problem.Code != "ssh_auth" || vault.writes != 0 {
		t.Fatalf("invalid credentials reached storage: %+v", r)
	}
	input.Password = "fixture-password"
	if r := b.ConnectSSH(input); r.Problem != nil || r.Warning != nil {
		t.Fatalf("enroll: %+v", r)
	}
	s.Close()
	vault.password = "outdated-fixture-password"
	fresh := NewSession(options)
	defer fresh.Close()
	fb := NewBridge(fresh)
	if r := fb.Start(); r.Problem == nil || r.Problem.Code != "ssh_auth" {
		t.Fatalf("bad saved password accepted: %+v", r)
	}
	// A manually supplied password bypasses an unavailable vault and stays
	// transient unless remembering is explicitly requested again.
	vault.fail = true
	reads, writes := vault.reads, vault.writes
	if r := fb.UseConnection(SelectConnection{Name: input.Name, Password: input.Password}); r.Problem != nil || r.Warning != nil {
		t.Fatalf("manual override: %+v", r)
	}
	if vault.reads != reads || vault.writes != writes || vault.password != "outdated-fixture-password" {
		t.Fatal("transient override accessed or replaced saved secret")
	}
	vault.fail = false
	if r := fb.UseConnection(SelectConnection{Name: input.Name, Password: input.Password, RememberPassword: true}); r.Problem != nil || r.Warning != nil || vault.password != input.Password {
		t.Fatalf("explicit saved password replacement: %+v", r)
	}
	var output bytes.Buffer
	app := cli.App{Out: &output, Err: &output, PasswordVault: vault}
	reads, writes = vault.reads, vault.writes
	for _, args := range [][]string{
		{"context", "password", "save", input.Name},
		{"context", "password", "invalid", input.Name},
		{"--context", input.Name, "context", "password", "status", input.Name},
	} {
		if err := app.Run(t.Context(), append([]string{"--data-dir", local.Dir}, args...)); err == nil {
			t.Fatal("invalid CLI password request accepted", args)
		}
	}
	if err := app.Run(t.Context(), []string{"--data-dir", local.Dir, "context", "password", "status", input.Name}); err != nil {
		t.Fatal("CLI password metadata", err)
	}
	if vault.reads != reads || vault.writes != writes {
		t.Fatal("CLI invalid request or metadata accessed secrets")
	}
}
