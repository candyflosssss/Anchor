package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

type catalogTransport func(*http.Request) (*http.Response, error)

func (f catalogTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCatalogBridgeClientCoreAndCLI(t *testing.T) {
	dir := t.TempDir()
	st, e := state.Open(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	raw, e := os.ReadFile("../modeldev/testdata/catalog.json")
	if e != nil {
		t.Fatal(e)
	}
	var calls atomic.Int32
	catalog := modeldev.NewWithTransport(catalogTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.String() != modeldev.Endpoint || r.Header.Get("Authorization") != "" {
			t.Error("unexpected catalog credentials")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
	}))
	srv := core.NewWithModelCatalog(st, slog.New(slog.NewJSONHandler(io.Discard, nil)), catalog)
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	cfg, e := st.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860")
	if e != nil {
		t.Fatal(e)
	}
	session := NewSession(Options{Dir: dir})
	defer session.Close()
	session.cfg = &cfg
	b := NewBridge(session)
	q := ModelCatalogQuery{ProviderID: "lab", ModelID: "vision-reasoner"}
	out := b.ModelCatalog(q)
	if out.Problem != nil || out.Data == nil || out.Data.Models[0].Limits.Context == nil || *out.Data.Models[0].Limits.Context != 400000 {
		t.Fatal(out)
	}
	v := func(n int64) *int64 { return &n }
	estimate := b.EstimateModelCost(CatalogEstimateInput{Query: q, Usage: CatalogUsage{InputTokens: v(1000), OutputTokens: v(100), CacheReadTokens: v(400), CacheWriteTokens: v(100), ReasoningTokens: v(30)}})
	if estimate.Problem != nil || estimate.Data == nil || estimate.Data.USD == nil || *estimate.Data.USD != "0.00265" {
		t.Fatal(estimate)
	}
	missing := b.ModelCatalog(ModelCatalogQuery{ProviderID: "lab", ModelID: "not-listed"})
	if missing.Data != nil || missing.Problem == nil {
		t.Fatal(missing)
	}
	run := func(wantErr bool, args ...string) string {
		t.Helper()
		var output bytes.Buffer
		app := cli.App{Out: &output, Err: io.Discard, In: strings.NewReader("")}
		e := app.Run(context.Background(), append([]string{"--data-dir", dir}, args...))
		if (e != nil) != wantErr {
			t.Fatalf("%v: %v", args, e)
		}
		return output.String()
	}
	run(false, "context", "add", "local", "--local")
	run(false, "context", "use", "local")
	if got := run(false, "catalog", "providers"); !strings.Contains(got, `"id":"lab"`) {
		t.Fatal(got)
	}
	if got := run(false, "catalog", "models", "--provider", "lab", "--search", "VISION"); !strings.Contains(got, `"total":1`) {
		t.Fatal(got)
	}
	if got := run(false, "catalog", "model", "--provider", "lab", "--model", "vision-reasoner"); !strings.Contains(got, `"reasoning_options"`) {
		t.Fatal(got)
	}
	got := run(false, "catalog", "estimate", "--provider", "lab", "--model", "vision-reasoner", "--input-tokens", "1000", "--output-tokens", "100", "--cache-read-tokens", "400", "--cache-write-tokens", "100", "--reasoning-tokens", "30")
	var priced modeldev.CatalogEstimate
	if json.Unmarshal([]byte(got), &priced) != nil || priced.USD == nil || *priced.USD != "0.00265" {
		t.Fatal(got)
	}
	if got := run(false, "catalog", "estimate", "--provider", "lab", "--model", "vision-reasoner"); !strings.Contains(got, `"reason":"missing_usage"`) {
		t.Fatal(got)
	}
	run(true, "catalog", "model", "--provider", "lab")
	run(true, "catalog", "models", "--provider", "lab", "--limit", "101")
	run(true, "catalog", "estimate", "--provider", "lab", "--model", "vision-reasoner", "--input-tokens", "-2")
	if calls.Load() != 1 {
		t.Fatal("shared catalog cache", calls.Load())
	}
}
