package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestHelpDocumentsDistinctAgentFailureCategories(t *testing.T) {
	var out bytes.Buffer
	a := App{In: strings.NewReader(""), Out: &out, Err: io.Discard}
	if err := a.Run(context.Background(), []string{"help"}); err != nil {
		t.Fatal(err)
	}
	for _, hint := range []string{"Agent failures distinguish revision conflicts", "external configuration edits", "unavailable default models and invalid catalogs", "unclassified failures remain agent_operation_failed"} {
		if !strings.Contains(out.String(), hint) {
			t.Fatal("missing failure diagnostic help", hint)
		}
	}
}
