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

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestProviderChatCLI(t *testing.T) {
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
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Reasoning struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
			Input []catalog.ChatMessage `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Reasoning.Effort != "xhigh" {
			t.Error("CLI lost reasoning effort")
		}
		if len(body.Input) != 3 {
			t.Error("CLI lost history")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"CLI OK\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	defer up.Close()
	secret := "synthetic-cli-key"
	if _, err = store.PutProvider(state.Provider{ID: "p", BaseURL: up.URL, Enabled: true, Protocol: "responses", TimeoutSeconds: 5, MaxConcurrent: 1}, 0, &secret); err != nil {
		t.Fatal(err)
	}
	var setupOut bytes.Buffer
	setupApp := App{In: strings.NewReader(""), Out: &setupOut, Err: &setupOut}
	for _, args := range [][]string{{"context", "add", "local", "--local"}, {"context", "use", "local"}} {
		if err := setupApp.Run(context.Background(), append([]string{"--data-dir", dir}, args...)); err != nil {
			t.Fatal(err)
		}
	}
	var out, diagnostics bytes.Buffer
	app := App{In: strings.NewReader(`[{"role":"user","content":"first"},{"role":"assistant","content":"second"},{"role":"user","content":"third"}]`), Out: &out, Err: &diagnostics}
	err = app.Run(context.Background(), []string{"--data-dir", dir, "provider", "chat", "--id", "p", "--revision", "1", "--key", "default", "--model", "demo", "--reasoning-effort", "xhigh", "--approve"})
	if err != nil {
		t.Fatal(err, diagnostics.String())
	}
	var progress catalog.ProbeProgress
	if json.Unmarshal(out.Bytes(), &progress) != nil || progress.Text != "CLI OK" || progress.Result.Failure != "" {
		t.Fatal(out.String())
	}
}
func TestProviderChatCLIRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		input string
		args  []string
	}{
		{`[{"role":"user","content":"hello"}]`, []string{"--id", "p", "--revision", "1", "--key", "default", "--model", "m", "--approve", "--reasoning-effort", "invalid"}},
		{`[{"role":"user","content":"hello"}]`, []string{"--id", "p", "--revision", "1", "--key", "default", "--model", "m"}},
		{`[{"role":"system","content":"hello"}]`, []string{"--id", "p", "--revision", "1", "--key", "default", "--model", "m", "--approve"}},
		{`[] true`, []string{"--id", "p", "--revision", "1", "--key", "default", "--model", "m", "--approve"}},
	} {
		var out bytes.Buffer
		app := App{In: strings.NewReader(tc.input), Out: &out, Err: &out}
		if app.providerChat(context.Background(), nil, tc.args) == nil {
			t.Fatal("invalid input accepted")
		}
	}
}
