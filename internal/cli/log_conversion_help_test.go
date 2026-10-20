package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestLogConversionDiagnosticsHelp(t *testing.T) {
	var out bytes.Buffer
	if err := (App{Out: &out, Err: io.Discard}).Run(t.Context(), []string{"help"}); err != nil {
		t.Fatal(err)
	}
	for _, hint := range []string{"Conversion failures retain route IDs", "conversion_target/conversion_reason", "detail/export"} {
		if !strings.Contains(out.String(), hint) {
			t.Fatal("missing safe conversion diagnostic help", hint)
		}
	}
}
