package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestContextBootstrapGuardsBeforeInitialization(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	app := App{In: strings.NewReader(""), Out: &out, Err: &out}
	for _, args := range [][]string{{"context", "bootstrap"}, {"--context", "remote", "context", "bootstrap", "--show-once"}, {"context", "bootstrap", "--show-once", "--control-port", "0"}} {
		if err := app.Run(context.Background(), append([]string{"--data-dir", dir}, args...)); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if _, err := state.Open(dir, false); err == nil {
		t.Fatal("invalid bootstrap initialized state")
	}
}
func TestContextAddRequiresOneTransport(t *testing.T) {
	s, err := state.Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.Initialize("127.0.0.1:17861", "127.0.0.1:17860")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	app := App{Out: &out, Err: &out}
	for _, args := range [][]string{{"add", "remote"}, {"add", "remote", "--local", "--ssh", "user@host"}, {"add", "local", "--local", "--trust-new-host"}} {
		if err := app.contexts(t.Context(), s, args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	profiles, _ := s.Contexts()
	if len(profiles) != 0 {
		t.Fatal("invalid add persisted a profile")
	}
}
