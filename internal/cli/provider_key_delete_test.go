package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestCLIKeyDeletionPreservesOtherModelSources(t *testing.T) {
	dir := t.TempDir()
	s, err := state.Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	srv := core.New(s, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	if _, err = s.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	run := func(input string, wantErr bool, args ...string) []byte {
		t.Helper()
		var out, diagnostics bytes.Buffer
		a := App{In: strings.NewReader(input), Out: &out, Err: &diagnostics}
		err := a.Run(context.Background(), append([]string{"--data-dir", dir}, args...))
		if (err != nil) != wantErr {
			t.Fatalf("%v: error=%v diagnostics=%s", args, err, &diagnostics)
		}
		if !wantErr && !json.Valid(out.Bytes()) {
			t.Fatalf("%v: invalid output %s", args, &out)
		}
		if strings.Contains(out.String(), "synthetic-key") {
			t.Fatal("secret leaked")
		}
		return out.Bytes()
	}
	run("", false, "context", "add", "local", "--local")
	run("", false, "context", "use", "local")
	for _, id := range []string{"p", "other"} {
		run("synthetic-key\n", false, "provider", "add", "--id", id, "--base-url", "http://127.0.0.1:1/v1", "--secret-stdin")
	}
	run("synthetic-key\n", false, "provider", "key-put", "--id", "p", "--revision", "1", "--target", "backup", "--secret-stdin")
	for _, id := range []string{"only", "shared-keys", "shared-provider"} {
		args := []string{"provider", "model-put", "--id", "p", "--revision", "2", "--target", id, "--model", "same-upstream", "--source-key", "default"}
		if id == "shared-keys" {
			args = append(args, "--source-key", "backup")
		}
		run("", false, args...)
	}
	run("", false, "provider", "model-put", "--id", "other", "--revision", "1", "--target", "shared-provider", "--model", "same-upstream", "--source-key", "default")
	remove := []string{"provider", "key-delete", "--id", "p", "--revision", "2", "--target", "default"}
	run("", true, remove...)
	var workspace catalog.Workspace
	if err = json.Unmarshal(run("", false, append(remove, "--approve")...), &workspace); err != nil {
		t.Fatal(err)
	}
	if workspace.Revision != 3 || len(workspace.Keys) != 1 || workspace.Keys[0].ID != "backup" || len(workspace.Models) != 1 || workspace.Models[0].ID != "shared-keys" || !reflect.DeepEqual(workspace.Models[0].KeyIDs, []string{"backup"}) {
		t.Fatalf("incorrect committed workspace: %+v", workspace)
	}
	if _, err = s.Model("only"); !errors.Is(err, state.ErrNotFound) {
		t.Fatal("orphan retained", err)
	}
	shared, err := s.Model("shared-provider")
	if err != nil || len(shared.Candidates) != 1 || shared.Candidates[0].ProviderID != "other" || !reflect.DeepEqual(shared.Candidates[0].KeyIDs, []string{"default"}) {
		t.Fatal("other provider's same-named source changed", shared, err)
	}
	// A response from the committed write is safe to use as the next revision;
	// reusing the previous revision must fail without deleting the other Key.
	run("", true, "provider", "key-delete", "--id", "p", "--revision", "2", "--target", "backup", "--approve")
	if keys, err := s.ProviderKeys("p"); err != nil || len(keys) != 1 {
		t.Fatal("stale deletion changed Keys", keys, err)
	}
}

func TestKeyDeletionHelp(t *testing.T) {
	var out bytes.Buffer
	a := App{In: strings.NewReader(""), Out: &out, Err: io.Discard}
	if err := a.Run(context.Background(), []string{"help"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "key-delete removes only this Key's mapping sources; other Keys and providers keep shared models.") {
		t.Fatal("key deletion side effects missing from help")
	}
}
