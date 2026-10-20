package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestUpdateLocalCLI(t *testing.T) {
	dir := t.TempDir()
	var out, diagnostics bytes.Buffer
	app := App{In: strings.NewReader(""), Out: &out, Err: &diagnostics}
	run := func(args ...string) error {
		out.Reset()
		return app.Run(context.Background(), append([]string{"--data-dir", dir}, args...))
	}
	if err := run("update", "config"); err != nil {
		t.Fatal(err)
	}
	var cfg map[string]string
	if err := json.Unmarshal(out.Bytes(), &cfg); err != nil || cfg["channel"] != "alpha" {
		t.Fatal(cfg, err)
	}
	if err := run("update", "configure", "--channel", "dev", "--manifest", "https://example.invalid/dev.json"); err != nil {
		t.Fatal(err)
	}
	if err := run("update", "configure", "--channel", "alpha", "--default-source"); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &cfg); err != nil || cfg["manifest_url"] != "" || cfg["channel"] != "alpha" {
		t.Fatal(cfg, err)
	}
	for _, args := range [][]string{
		{"--context", "remote", "update", "config"}, {"update", "apply"}, {"update", "apply", "--approve", "extra"},
		{"update", "configure", "--channel", "nightly"}, {"update", "configure", "--manifest", "http://example.com/feed"},
		{"update", "configure", "--manifest", "https://example.invalid/feed", "--default-source"},
		{"update", "check", "--product", "desktop"}, {"update", "check", "--current-version", "1.0.0"},
		{"update", "upgrade"}, {"update", "desktop"}, {"update", "upgrade", "--approve", "--expected-version", "1.0.0"},
		{"--context", "remote", "update", "upgrade"}, {"update", "config", "extra"}, {"update", "unknown"},
	} {
		if err := run(args...); err == nil {
			t.Fatal("invalid update command accepted", args)
		}
	}
	if err := run("help"); err != nil || !strings.Contains(out.String(), "update apply --approve") {
		t.Fatal("missing update help", err)
	}
}
