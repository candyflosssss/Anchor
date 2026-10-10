package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestInitIdempotentAndLocalOnly(t *testing.T) {
	dir := t.TempDir()
	var out, diagnostics bytes.Buffer
	a := App{In: strings.NewReader(""), Out: &out, Err: &diagnostics}
	if e := a.Run(context.Background(), []string{"init", "--data-dir", dir}); e != nil {
		t.Fatal(e)
	}
	first := out.String()
	if strings.Contains(first, "token") {
		t.Fatal("init leaked credential")
	}
	out.Reset()
	if e := a.Run(context.Background(), []string{"--data-dir", dir, "init"}); e != nil {
		t.Fatal(e)
	}
	if first != out.String() {
		t.Fatal("init rotated state")
	}
	if e := a.Run(context.Background(), []string{"--data-dir", dir, "--context", "remote", "serve"}); e == nil {
		t.Fatal("context accepted for local-only serve")
	}
}
func TestCLIManagementWorkflow(t *testing.T) {
	dir := t.TempDir()
	s, e := state.Open(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	srv := core.New(s, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	host := strings.TrimPrefix(web.URL, "http://")
	if _, e = s.Initialize(host, "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	run := func(input string, args ...string) string {
		t.Helper()
		var out, errout bytes.Buffer
		a := App{In: strings.NewReader(input), Out: &out, Err: &errout}
		args = append([]string{"--data-dir", dir}, args...)
		if e := a.Run(context.Background(), args); e != nil {
			t.Fatal(args, e, errout.String())
		}
		if errout.Len() != 0 {
			t.Fatal("unexpected diagnostics", errout.String())
		}
		var v any
		if e := json.Unmarshal(out.Bytes(), &v); e != nil {
			t.Fatal("stdout is not JSON", out.String())
		}
		return out.String()
	}
	run("", "context", "add", "local", "--local")
	run("", "context", "use", "local")
	if status := run("", "status", "--json"); !strings.Contains(status, `"core":"ready"`) {
		t.Fatal(status)
	}
	out := run("provider-secret\n", "provider", "add", "--id", "p", "--base-url", "https://example.invalid/v1", "--secret-stdin")
	if strings.Contains(out, "provider-secret") {
		t.Fatal("provider write leaked secret")
	}
	run("", "model", "add", "--id", "public", "--candidate", "p:actual", "--tools", "--streaming")
	run("", "provider", "list")
	run("", "model", "list")
	run("", "context", "test", "local")
	grant := run("", "access", "grant", "--label", "diagnostic", "--scope", "gateway", "--show-once")
	var data struct {
		Access state.Access `json:"access"`
		Token  string       `json:"token"`
	}
	if e = json.Unmarshal([]byte(grant), &data); e != nil || data.Token == "" {
		t.Fatal("grant missing token", e)
	}
	if listed := run("", "access", "list"); strings.Contains(listed, data.Token) {
		t.Fatal("access list leaked token")
	}
	run("", "access", "revoke", data.Access.ID)
	if s.Authenticate(data.Token, "gateway") {
		t.Fatal("CLI revoke did not work")
	}
}
func TestCLIRequiresExplicitSecretDisclosure(t *testing.T) {
	dir := t.TempDir()
	s, e := state.Open(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Initialize("127.0.0.1:17861", "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	var out bytes.Buffer
	a := App{In: strings.NewReader(""), Out: &out, Err: io.Discard}
	if e = a.Run(context.Background(), []string{"--data-dir", dir, "access", "grant", "--label", "test", "--scope", "gateway"}); e == nil {
		t.Fatal("grant disclosed without explicit flag")
	}
	if out.Len() != 0 {
		t.Fatal("failed grant printed token")
	}
}
func TestCLIUnknownContextDoesNotFallback(t *testing.T) {
	dir := t.TempDir()
	s, e := state.Open(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Initialize("127.0.0.1:17861", "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	a := App{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard}
	if e = a.Run(context.Background(), []string{"--data-dir", dir, "--context", "unknown", "status"}); e == nil {
		t.Fatal("unknown context fell back")
	}
}
func TestCoreRunAndCancelClosesListeners(t *testing.T) {
	// Reserve ephemeral ports only long enough to choose the test addresses.
	free := func() string {
		ln, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		address := ln.Addr().String()
		ln.Close()
		return address
	}
	s, e := state.Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	control, gateway := free(), free()
	if _, e = s.Initialize(control, gateway); e != nil {
		t.Fatal(e)
	}
	srv := core.New(s, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = srv.Run(ctx); e != nil {
		t.Fatal(e)
	}
	for _, address := range []string{control, gateway} {
		ln, e := net.Listen("tcp", address)
		if e != nil {
			t.Fatal("listener leaked", address, e)
		}
		ln.Close()
	}
}

func TestVersionWithoutInitializedInstance(t *testing.T) {
	var out bytes.Buffer
	dir := t.TempDir()
	a := App{In: strings.NewReader(""), Out: &out, Err: io.Discard}
	if err := a.Run(context.Background(), []string{"--data-dir", dir, "version"}); err != nil {
		t.Fatal(err)
	}
	var value map[string]string
	if err := json.Unmarshal(out.Bytes(), &value); err != nil || value["version"] != core.Version || len(value) != 1 {
		t.Fatal("version output must match the linked build version", out.String(), err)
	}
}
