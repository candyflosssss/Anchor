package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestRestartCLIValidation(t *testing.T) {
	for _, args := range [][]string{
		{"restart", "--approve", "--expected-instance", "bad id"},
		{"restart", "--approve", "--control-port", "17861"},
		{"restart", "--approve", "--control-port", "-1"},
		{"restart", "--approve", "--control-port", "65536"},
		{"--context", "remote", "restart", "--approve", "--expected-instance", "fixture-instance"},
		{"restart"}, {"restart", "--approve", "unexpected"}, {"restart", "--approve", "--timeout", "0s"}, {"restart", "--approve", "--timeout", "31s"}, {"restart", "--approve", "--unknown"}, {"--context", "remote", "restart", "--approve"}} {
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
	for _, text := range []string{"restart --approve [--timeout 30s]", "explicit --context SSH target", "--expected-instance ID", "--control-port N"} {
		if !strings.Contains(help.String(), text) {
			t.Fatalf("restart help missing %q", text)
		}
	}
}
