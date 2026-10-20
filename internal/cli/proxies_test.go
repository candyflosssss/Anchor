package cli

import (
	"bytes"
	"context"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyCLIWorkflow(t *testing.T) {
	dir := t.TempDir()
	s, e := state.Open(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	srv := core.New(s, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	if _, e = s.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	run := func(input string, wantErr bool, args ...string) string {
		t.Helper()
		var out, diag bytes.Buffer
		a := App{In: strings.NewReader(input), Out: &out, Err: &diag}
		e := a.Run(context.Background(), append([]string{"--data-dir", dir}, args...))
		if (e != nil) != wantErr {
			t.Fatalf("%v: %v", args, e)
		}
		if strings.Contains(out.String(), "cli-proxy-secret") {
			t.Fatal("secret leaked")
		}
		return out.String()
	}
	run("", false, "context", "add", "local", "--local")
	run("", false, "context", "use", "local")
	if out := run("", false, "proxy", "list"); !strings.Contains(out, `"revision":1`) {
		t.Fatal(out)
	}
	run("http://user:cli-proxy-secret@127.0.0.1:1234\n", false, "proxy", "put", "edge", "--name", "Edge", "--kind", "http", "--secret-stdin", "--revision", "1", "--approve")
	run("", true, "proxy", "delete", "edge", "--revision", "1", "--approve")
	run("", true, "proxy", "delete", "edge", "--revision", "2")
	run("", false, "proxy", "delete", "edge", "--revision", "2", "--approve")
	secret := "synthetic"
	if _, e = s.PutProvider(state.Provider{ID: "p", BaseURL: "https://upstream.example", Protocol: "responses", Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 1}, 0, &secret); e != nil {
		t.Fatal(e)
	}
	if out := run("", false, "proxy", "bind", "p", "--mode", "system", "--revision", "3", "--approve"); !strings.Contains(out, `"mode":"system"`) {
		t.Fatal(out)
	}
	run("", true, "proxy", "bind", "p", "--mode", "system", "--proxy", "edge", "--revision", "4", "--approve")
	run("", true, "proxy", "bind", "p", "--mode", "direct", "--revision", "3", "--approve")
	run("", false, "proxy", "bind", "p", "--key", "default", "--mode", "direct", "--revision", "4", "--approve")
}
