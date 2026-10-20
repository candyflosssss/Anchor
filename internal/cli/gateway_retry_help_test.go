package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHelpDocumentsUnifiedCodexRetries(t *testing.T) {
	var out bytes.Buffer
	if err := (App{Out: &out}).Run(context.Background(), []string{"--help"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"one ordinary attempt per channel", "Codex owns request/stream retries (default 5)", "agent codex-settings [--retries 0..10 --revision N]", "no replay after output or unknown execution", "management writes are not retried", "Logs mark encrypted-context cleanup", "native Responses may retry once before output after a definite encrypted-content rejection"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help missing %q", want)
		}
	}
}
