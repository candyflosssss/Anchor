package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestModelCreationHelpDocumentsCatalogDefaults(t *testing.T) {
	for _, command := range []string{"add", "put"} {
		var out bytes.Buffer
		app := App{Err: &out}
		err := app.models(context.Background(), nil, []string{command, "--help"})
		if !errors.Is(err, flag.ErrHelp) || !strings.Contains(out.String(), "models.dev defaults") {
			t.Fatal(command, err, out.String())
		}
	}
}
