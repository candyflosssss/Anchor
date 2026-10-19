package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestProviderProtocolCLIEdit(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := core.New(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	if _, err = store.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		var out, diagnostics bytes.Buffer
		app := App{In: strings.NewReader("synthetic-cli-protocol"), Out: &out, Err: &diagnostics}
		err := app.Run(context.Background(), append([]string{"--data-dir", dir}, args...))
		return out.String() + diagnostics.String(), err
	}
	for _, args := range [][]string{{"context", "add", "local", "--local"}, {"context", "use", "local"}} {
		if _, err := run(args...); err != nil {
			t.Fatal(err)
		}
	}
	check := func(want string, args ...string) {
		t.Helper()
		raw, err := run(args...)
		var p state.Provider
		if err != nil || json.Unmarshal([]byte(raw), &p) != nil || p.Protocol != want {
			t.Fatalf("%v: %s %v", args, raw, err)
		}
		keys, err := store.ProviderKeys("p")
		if err != nil || len(keys) != 1 || keys[0].Protocol != want || keys[0].Secret != "synthetic-cli-protocol" {
			t.Fatal(keys, err)
		}
		if strings.Contains(raw, "synthetic-cli-protocol") {
			t.Fatal("secret leaked")
		}
	}
	check("anthropic", "provider", "add", "--id", "p", "--base-url", "https://example.invalid/v1", "--protocol", "anthropic", "--secret-stdin")
	check("chat-completions", "provider", "put", "--id", "p", "--base-url", "https://example.invalid/v1", "--revision", "1", "--protocol", "chat-completions")
	check("chat-completions", "provider", "put", "--id", "p", "--base-url", "https://example.invalid/v1", "--revision", "2")
	for _, args := range [][]string{
		{"provider", "put", "--id", "p", "--base-url", "https://example.invalid/v1", "--revision", "1", "--protocol", "responses"},
		{"provider", "put", "--id", "p", "--base-url", "https://example.invalid/v1", "--revision", "3", "--protocol", "unsupported"},
	} {
		if _, err := run(args...); err == nil {
			t.Fatalf("accepted invalid or stale write: %v", args)
		}
	}
	p, _ := store.Provider("p")
	keys, _ := store.ProviderKeys("p")
	if p.Revision != 3 || p.Protocol != "chat-completions" || keys[0].Protocol != "chat-completions" {
		t.Fatal("failed write mutated protocols")
	}
	raw, _ := run("provider", "put", "--help")
	if !strings.Contains(raw, "default Key; omitted keeps existing protocols") {
		t.Fatal("missing protocol help", raw)
	}
}
