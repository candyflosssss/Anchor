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

func TestModelHelpDocumentsStreamingDefault(t *testing.T) {
	for _, command := range []string{"model add", "model put", "provider model-put"} {
		t.Run(command, func(t *testing.T) {
			var out bytes.Buffer
			app := App{Err: &out}
			parts := strings.Fields(command)
			args := []string{parts[1], "--help"}
			var err error
			if parts[0] == "model" {
				err = app.models(context.Background(), nil, args)
			} else {
				err = app.providerFeature(context.Background(), nil, args)
			}
			if !errors.Is(err, flag.ErrHelp) {
				t.Fatal(err)
			}
			start := strings.Index(out.String(), "  -streaming")
			if start < 0 {
				t.Fatal(out.String())
			}
			section := out.String()[start:]
			if end := strings.Index(section[1:], "\n  -"); end >= 0 {
				section = section[:end+1]
			}
			if !strings.Contains(section, "default true") || !strings.Contains(section, "--streaming=false") {
				t.Fatal(section)
			}
		})
	}
}

func TestModelSettingsHelpDocumentsPriceAwareDefaults(t *testing.T) {
	var out bytes.Buffer
	app := App{Err: &out}
	err := app.models(context.Background(), nil, []string{"settings", "--help"})
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(out.String(), "price-aware") || !strings.Contains(out.String(), "context_policy") || !strings.Contains(out.String(), "no write") {
		t.Fatal(err, out.String())
	}
}
