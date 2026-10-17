package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHelpDocumentsDefaultChannelRetries(t *testing.T) {
	var out bytes.Buffer
	if err := (App{Out: &out}).Run(context.Background(), []string{"--help"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"3 same-channel retries (4 attempts)", "502/503/504", "pre-connection failures", "250ms/500ms/1s", "provider timeout", "channel_retries: 0..10", "omitted/null preserves", "capped at 1s", "no replay after output or unknown execution", "401/403/429", "management writes are not retried"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help missing %q: %s", want, out.String())
		}
	}
}
