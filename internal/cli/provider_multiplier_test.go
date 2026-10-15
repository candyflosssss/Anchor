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

func TestProviderCostMultiplierCLI(t *testing.T) {
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
	if _, err := store.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		var out, diagnostics bytes.Buffer
		app := App{In: strings.NewReader(""), Out: &out, Err: &diagnostics}
		err := app.Run(context.Background(), append([]string{"--data-dir", dir}, args...))
		return out.String(), err
	}
	for _, args := range [][]string{{"context", "add", "local", "--local"}, {"context", "use", "local"}} {
		if _, err := run(args...); err != nil {
			t.Fatal(err)
		}
	}
	check := func(want float64, args ...string) {
		t.Helper()
		raw, err := run(args...)
		var p state.Provider
		if err != nil || json.Unmarshal([]byte(raw), &p) != nil || p.CostMultiplier != want {
			t.Fatalf("%v: %s, %v", args, raw, err)
		}
	}
	check(1, "provider", "add", "--id", "p", "--base-url", "https://example.invalid/v1")
	check(.5, "provider", "put", "--id", "p", "--base-url", "https://example.invalid/v1", "--revision", "1", "--cost-multiplier", "0.5")
	check(.5, "provider", "put", "--id", "p", "--base-url", "https://example.invalid/v1", "--revision", "2")
	for _, invalid := range []string{"-1", "NaN", "Inf", "1001", "0.0000001"} {
		if _, err := run("provider", "put", "--id", "p", "--base-url", "https://example.invalid/v1", "--revision", "3", "--cost-multiplier", invalid); err == nil {
			t.Fatalf("accepted %s", invalid)
		}
	}
	p, _ := store.Provider("p")
	if p.Revision != 3 || p.CostMultiplier != .5 {
		t.Fatal("invalid save mutated provider", p)
	}
}
