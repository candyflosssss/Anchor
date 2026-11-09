package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestKeyCooldownHelpDescribesFallback(t *testing.T) {
	var out bytes.Buffer
	a := App{Out: &out, Err: &out}
	if err := a.Run(t.Context(), []string{"--help"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Cooling Keys remain low-priority fallbacks") {
		t.Fatal("help does not describe cooldown scheduling", out.String())
	}
}
