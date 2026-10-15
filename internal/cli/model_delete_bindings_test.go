package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestCLIModelDeleteWithDisabledBinding(t *testing.T) {
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
	run := func(wantCode string, args ...string) {
		t.Helper()
		var out, diagnostics bytes.Buffer
		a := App{In: strings.NewReader(""), Out: &out, Err: &diagnostics}
		err := a.Run(context.Background(), append([]string{"--data-dir", dir}, args...))
		if wantCode != "" {
			var requestErr *client.RequestError
			if !errors.As(err, &requestErr) || requestErr.Code != wantCode || requestErr.Status != http.StatusConflict || requestErr.Unknown {
				t.Fatalf("%v: expected known HTTP 409 %s, got %v", args, wantCode, err)
			}
			return
		}
		if err != nil || diagnostics.Len() != 0 || !json.Valid(out.Bytes()) {
			t.Fatalf("%v: err=%v diagnostics=%s output=%s", args, err, &diagnostics, &out)
		}
	}
	run("", "context", "add", "local", "--local")
	run("", "context", "use", "local")
	run("", "provider", "add", "--id", "p", "--base-url", "http://127.0.0.1:1/v1")
	run("", "model", "add", "--id", "alias", "--candidate", "p:up")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.AddBinding(state.Binding{ID: "agent", AdapterID: "claude", AdapterVersion: "1", ConfigRoot: t.TempDir(), Executable: exe, Model: "alias", ModelScope: "all-mapped", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.BindingToken(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	remove := []string{"provider", "model-delete", "--id", "p", "--revision", "1", "--target", "alias", "--model-revision", "1", "--approve"}
	run("referenced", remove...)
	run("", "agent", "disable", b.ID, "--revision", "1")
	disabled, err := s.Binding(b.ID)
	if err != nil || disabled.Enabled || disabled.Revision != 2 {
		t.Fatalf("binding not disabled: %+v, %v", disabled, err)
	}
	run("", remove...)
	if _, err = s.Model("alias"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("model not deleted: %v", err)
	}
	run("agent_model_unavailable", "agent", "enable", b.ID, "--revision", "2")
	if got, err := s.Binding(b.ID); err != nil || !reflect.DeepEqual(got, disabled) {
		t.Fatalf("deletion or rejected enable changed binding: %+v, %v", got, err)
	}
	if _, err = s.BindingForToken(token); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("orphaned binding credential enabled: %v", err)
	}
	// Selecting a replacement repairs the saved policy without silently enabling it.
	run("", "model", "add", "--id", "replacement", "--candidate", "p:up")
	run("", "agent", "set-models", b.ID, "--revision", "2", "--model", "replacement", "--model-scope", "all-mapped")
	repaired, err := s.Binding(b.ID)
	if err != nil || repaired.Enabled || repaired.Model != "replacement" || repaired.Revision != 3 {
		t.Fatalf("replacement policy not saved: %+v, %v", repaired, err)
	}
	run("", "agent", "enable", b.ID, "--revision", strconv.FormatInt(repaired.Revision, 10))
	if enabled, err := s.BindingForToken(token); err != nil || !enabled.Enabled || enabled.Model != "replacement" {
		t.Fatalf("repaired binding not enabled with original credential: %+v, %v", enabled, err)
	}
	entries, err := os.ReadDir(b.ConfigRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("configuration was unexpectedly applied: entries=%v, err=%v", entries, err)
	}
}

func TestModelDeletionBindingPolicyHelp(t *testing.T) {
	var out bytes.Buffer
	a := App{In: strings.NewReader(""), Out: &out, Err: io.Discard}
	if err := a.Run(context.Background(), []string{"help"}); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{
		"Model deletion protects enabled Binding defaults; disabled Bindings do not block deletion.",
		"Agent enable requires an existing default model; use agent set-models to replace a removed default.",
	} {
		if !strings.Contains(out.String(), text) {
			t.Fatalf("help is missing %q", text)
		}
	}
}
