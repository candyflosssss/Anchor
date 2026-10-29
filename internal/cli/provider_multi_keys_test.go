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

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestProviderMultiKeyAddressCLI(t *testing.T) {
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
	run := func(args ...string) string {
		t.Helper()
		var out, diagnostics bytes.Buffer
		app := App{In: strings.NewReader("synthetic-cli-key"), Out: &out, Err: &diagnostics}
		if err := app.Run(context.Background(), append([]string{"--data-dir", dir}, args...)); err != nil {
			t.Fatal(args, err)
		}
		if strings.Contains(out.String()+diagnostics.String(), "synthetic-cli-key") {
			t.Fatal("secret leaked")
		}
		return out.String()
	}
	run("context", "add", "local", "--local")
	run("context", "use", "local")
	run("provider", "add", "--id", "p", "--base-url", "http://localhost:18080/v1", "--base-url", "https://192.0.2.10:8443/v1")
	run("provider", "key-put", "--id", "p", "--revision", "1", "--target", "named", "--protocol", "anthropic", "--secret-stdin")
	run("provider", "key-put", "--id", "p", "--revision", "2", "--target", "secondary", "--protocol", "chat-completions", "--secret-stdin")
	var rows []state.Provider
	if err = json.Unmarshal([]byte(run("provider", "list")), &rows); err != nil || len(rows) != 1 || !rows[0].HasSecret || rows[0].Revision != 3 || len(rows[0].BaseURLs) != 2 || rows[0].BaseURLs[0] != "http://localhost:18080/v1" {
		t.Fatal("named Key list status", err)
	}
	run("provider", "put", "--id", "p", "--revision", "3", "--base-url", "http://localhost:18081/v1")
	keys, err := store.ProviderKeys("p")
	if err != nil || len(keys) != 2 || keys[0].Protocol != "anthropic" || keys[1].Protocol != "chat-completions" || keys[0].Secret != "synthetic-cli-key" {
		t.Fatal("address edit changed Keys")
	}
	resetArgs := []string{"provider", "key-reset-all", "--id", "p", "--revision", "4"}
	var rejected bytes.Buffer
	app := App{Out: &rejected, Err: io.Discard}
	if err := app.Run(context.Background(), append([]string{"--data-dir", dir}, resetArgs...)); err == nil {
		t.Fatal("reset without approval succeeded")
	}
	var workspace catalog.Workspace
	if err := json.Unmarshal([]byte(run(append(resetArgs, "--approve")...)), &workspace); err != nil || workspace.Revision != 4 || len(workspace.Keys) != 2 {
		t.Fatal("reset all workspace", workspace, err)
	}
	for _, key := range workspace.Keys {
		if key.State != "ready" || key.Failures != 0 || key.ErrorScore != 0 || key.RetryAt != 0 || key.Test != nil {
			t.Fatal("key was not reset", key)
		}
	}
}
