package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/logbook"
)

func TestRealLogBridgeAndCLIKnownUsage(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/log-known-usage.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Contract string     `json:"contract"`
		Request  RequestLog `json:"request"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil || fixture.Contract != Contract {
		t.Fatal("fixture drift", err)
	}
	want := *fixture.Request.KnownUsage
	fixture.Request.StartedAtMS = time.Now().UnixMilli()
	s := testSession(t)
	logs, err := logbook.Open(s.options.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if !logs.Request(fixture.Request) {
		t.Fatal("seed rejected")
	}
	if err := logs.Close(); err != nil {
		t.Fatal(err)
	}
	b := NewBridge(s)
	requireReady(t, b)
	page := b.Logs(LogQuery{})
	if page.Problem != nil || page.Data == nil || len(page.Data.Requests) != 1 {
		t.Fatal(page)
	}
	r := page.Data.Requests[0]
	if len(r.Attempts) != 0 || r.KnownUsage == nil || *r.KnownUsage != want || r.Input != nil || r.Output != nil {
		t.Fatalf("list: %+v", r)
	}
	detail := b.LogDetail(r.ID)
	if detail.Problem != nil || detail.Data == nil || len(detail.Data.Attempts) != 2 || detail.Data.KnownUsage == nil || *detail.Data.KnownUsage != want || detail.Data.Input != nil || detail.Data.Output != nil {
		t.Fatal(detail)
	}
	if missing := b.LogDetail("missing-request"); missing.Problem == nil {
		t.Fatal("missing detail accepted")
	}
	run := func(args ...string) string {
		t.Helper()
		var output bytes.Buffer
		app := cli.App{Out: &output, Err: io.Discard, In: strings.NewReader("")}
		if err := app.Run(context.Background(), append([]string{"--data-dir", s.options.Dir}, args...)); err != nil {
			t.Fatal(args, err)
		}
		return output.String()
	}
	run("context", "add", "local", "--local")
	run("context", "use", "local")
	if help := run("help"); !strings.Contains(help, "known_usage {input, output, attempts}") || !strings.Contains(help, "unknown root totals stay null") {
		t.Fatal("usage subtotal help missing")
	}
	for _, args := range [][]string{{"logs", "list"}, {"logs", "detail", r.ID}, {"logs", "export"}} {
		got := run(args...)
		if !strings.Contains(got, `"known_usage"`) || !strings.Contains(got, `"input": 1200`) && !strings.Contains(got, `"input":1200`) {
			t.Fatalf("CLI %v lost subtotal: %s", args, got)
		}
	}
}
