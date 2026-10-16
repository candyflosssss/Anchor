package cli

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"strings"
	"testing"
)

func TestLogAttributionHelp(t *testing.T) {
	var out bytes.Buffer
	if err := (App{Out: &out, Err: io.Discard}).Run(t.Context(), []string{"help"}); err != nil {
		t.Fatal(err)
	}
	for _, hint := range []string{"native thread IDs and local project metadata", "explicit tags take precedence", "Historical logs are not backfilled"} {
		if !strings.Contains(out.String(), hint) {
			t.Fatal("missing attribution boundary", hint)
		}
	}
	for _, command := range []string{"list", "stats", "follow", "export"} {
		var help bytes.Buffer
		if err := (App{Err: &help}).logs(t.Context(), nil, []string{command, "--help"}); !errors.Is(err, flag.ErrHelp) {
			t.Fatal(err)
		}
		for _, hint := range []string{"explicit or Codex metadata", "explicit or Codex thread", "empty selects unattributed"} {
			if !strings.Contains(help.String(), hint) {
				t.Fatal(command, "missing log filter help", hint)
			}
		}
	}
}
