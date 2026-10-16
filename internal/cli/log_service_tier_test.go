package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestLogServiceTierCLI(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := core.New(store, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	if _, err = store.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","status":"completed","service_tier":"provider-turbo","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer up.Close()
	run := func(input string, args ...string) string {
		t.Helper()
		var out bytes.Buffer
		a := App{In: strings.NewReader(input), Out: &out, Err: io.Discard}
		if err := a.Run(context.Background(), append([]string{"--data-dir", dir}, args...)); err != nil {
			t.Fatal(args, err)
		}
		if strings.Contains(out.String(), "synthetic-cli-secret") {
			t.Fatal("credential in output")
		}
		return out.String()
	}
	run("", "context", "add", "local", "--local")
	run("", "context", "use", "local")
	run("synthetic-cli-secret", "provider", "add", "--id", "p", "--base-url", up.URL, "--secret-stdin")
	run("", "provider", "test", "--id", "p", "--revision", "1", "--target", "default", "--model", "demo", "--approve")
	var row logbook.Request
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		var page logbook.Page
		if err := json.Unmarshal([]byte(run("", "logs", "list", "--source", "probe")), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Requests) == 1 && page.Requests[0].Status == "succeeded" {
			row = page.Requests[0]
			break
		}
	}
	if row.ID == "" || row.Evidence["upstream_service_tier_raw"] != `"provider-turbo"` || row.Evidence["fast_triggered"] != "unknown" || len(row.Evidence) > 5 || len(row.Attempts) != 0 {
		t.Fatalf("invalid list evidence: %+v", row)
	}
	var suggestions logbook.Suggestions
	if err := json.Unmarshal([]byte(run("", "logs", "suggestions", "--field", "provider", "--prefix", "p", "--since", "1h", "--limit", "20")), &suggestions); err != nil || len(suggestions.Items) != 1 || suggestions.Items[0].Value != "p" || suggestions.Items[0].Count != 1 {
		t.Fatal(suggestions, err)
	}
	for _, args := range [][]string{{"--field", "document"}, {"--kind", "system", "--field", "key"}, {"--limit", "101"}, {"--since", "0s"}} {
		if err := (App{Out: io.Discard, Err: io.Discard}).Run(context.Background(), append([]string{"--data-dir", dir, "logs", "suggestions"}, args...)); err == nil {
			t.Fatal("invalid suggestions accepted", args)
		}
	}
	var stats logbook.Stats
	if err := json.Unmarshal([]byte(run("", "logs", "stats", "--source", "probe", "--since", "1h", "--interval", "1m")), &stats); err != nil || stats.Summary.Requests != 1 || len(stats.Buckets) != 60 {
		t.Fatal(stats, err)
	}
	for _, interval := range []string{"-1s", "13s", "1ns"} {
		if err := (App{Out: io.Discard, Err: io.Discard}).Run(context.Background(), []string{"--data-dir", dir, "logs", "stats", "--interval", interval}); err == nil {
			t.Fatal("invalid interval accepted", interval)
		}
	}
	var detail logbook.Request
	if err := json.Unmarshal([]byte(run("", "logs", "detail", row.ID)), &detail); err != nil || detail.Evidence["upstream_service_tier_raw"] != `"provider-turbo"` || len(detail.Attempts) != 1 {
		t.Fatal(detail, err)
	}
	if exported := run("", "logs", "export", "--source", "probe"); !strings.Contains(exported, "provider-turbo") {
		t.Fatal("export lost raw value")
	}
	if err := (App{Out: io.Discard, Err: io.Discard}).Run(context.Background(), []string{"--data-dir", dir, "logs", "detail", "missing-request"}); err == nil {
		t.Fatal("missing detail unexpectedly succeeded")
	}
}
