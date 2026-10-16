package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestLogDimensionCLI(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	logs, err := logbook.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []logbook.Request{{ID: "known", Agent: "codex", Binding: "desktop", Project: "anchor", Session: "thread-1"}, {ID: "legacy"}} {
		r.StartedAtMS = time.Now().UnixMilli()
		r.Source = "proxy"
		r.Status = "succeeded"
		logs.Request(r)
	}
	if err = logs.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	logs.Close()
	srv := core.New(store, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	if _, err = store.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		a := App{In: strings.NewReader(""), Out: &out, Err: io.Discard}
		err := a.Run(context.Background(), append([]string{"--data-dir", dir}, args...))
		return out.String(), err
	}
	for _, args := range [][]string{{"context", "add", "local", "--local"}, {"context", "use", "local"}} {
		if _, err = run(args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		args []string
		id   string
	}{
		{[]string{"logs", "list", "--agent", "codex", "--project", "anchor"}, "known"},
		{[]string{"logs", "list", "--binding", "desktop", "--session", "thread-1"}, "known"},
		{[]string{"logs", "list", "--agent", ""}, "legacy"},
	} {
		raw, err := run(tc.args...)
		var page logbook.Page
		if err != nil || json.Unmarshal([]byte(raw), &page) != nil || len(page.Requests) != 1 || page.Requests[0].ID != tc.id {
			t.Fatal(tc.args, raw, err)
		}
	}
	raw, err := run("logs", "stats", "--group-by", "project")
	var stats logbook.Stats
	if err != nil || json.Unmarshal([]byte(raw), &stats) != nil || stats.GroupBy != "project" || stats.GroupCount != 2 {
		t.Fatal(raw, err)
	}
	raw, err = run("logs", "export", "--project", "")
	if err != nil || !strings.Contains(raw, "legacy") || strings.Contains(raw, "known") {
		t.Fatal(raw, err)
	}
	raw, err = run("logs", "suggestions", "--field", "session")
	if err != nil || !strings.Contains(raw, "thread-1") {
		t.Fatal(raw, err)
	}
	if _, err = run("logs", "stats", "--group-by", "document"); err == nil {
		t.Fatal("invalid dimension accepted")
	}
	if _, err = run("logs", "list", "--kind", "system", "--agent", ""); err == nil {
		t.Fatal("system dimension filter accepted")
	}
}
