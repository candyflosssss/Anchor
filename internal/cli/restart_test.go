package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestRestartCLIValidation(t *testing.T) {
	for _, args := range [][]string{{"restart"}, {"restart", "--approve", "unexpected"}, {"restart", "--approve", "--timeout", "0s"}, {"restart", "--approve", "--timeout", "31s"}, {"restart", "--approve", "--unknown"}, {"--context", "remote", "restart", "--approve"}} {
		var stdout bytes.Buffer
		err := (App{Out: &stdout, Err: io.Discard}).Run(context.Background(), append([]string{"--data-dir", t.TempDir()}, args...))
		if err == nil || stdout.Len() != 0 {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
	var help bytes.Buffer
	if err := (App{Out: &help, Err: io.Discard}).Run(context.Background(), []string{"help"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(help.String(), "restart --approve [--timeout 30s]") {
		t.Fatal("restart missing from help")
	}
}
