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
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestLogReferenceCostCLI(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	srv := core.New(store, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer srv.Close()
	control := httptest.NewServer(srv.Control())
	defer control.Close()
	if _, err = store.Initialize(strings.TrimPrefix(control.URL, "http://"), "127.0.0.1:17860"); err != nil {
		t.Fatal(err)
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","status":"completed","output":[],"usage":{"input_tokens":1000,"output_tokens":100,"input_tokens_details":{"cached_tokens":400}}}`)
	}))
	defer up.Close()
	secret := "synthetic-price-key"
	if _, err = store.PutProvider(state.Provider{ID: "p", BaseURL: up.URL, Protocol: "responses", Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, 0, &secret); err != nil {
		t.Fatal(err)
	}
	model, err := modeldev.ParseModel([]byte(`{"p":{"models":{"priced":{"cost":{"input":2,"output":10,"cache_read":0.1}}}}}`), "p", "priced")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := modeldev.ModelCatalog{Source: "models.dev", Version: strings.Repeat("a", 64), FetchedAt: 1, ExpiresAt: 3601, Models: []modeldev.CatalogModel{model}, Total: 1}
	if _, err = store.PutModel(state.Model{ID: "public", Candidates: []state.Candidate{{ProviderID: "p", Model: "priced"}}, Settings: &state.ModelSettings{Reference: &snapshot}}, 0); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := (App{Out: &out, Err: io.Discard}).Run(t.Context(), append([]string{"--data-dir", dir}, args...)); err != nil {
			t.Fatal(args, err)
		}
		return out.String()
	}
	run("context", "add", "local", "--local")
	run("context", "use", "local")
	_, token, err := store.Grant("test", "gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	gateway := httptest.NewServer(srv.Gateway())
	defer gateway.Close()
	req, _ := http.NewRequestWithContext(t.Context(), "POST", gateway.URL+"/v1/responses", strings.NewReader(`{"model":"public","input":"synthetic"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal(resp.StatusCode)
	}
	var row logbook.Request
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		var page logbook.Page
		if err = json.Unmarshal([]byte(run("logs", "list")), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Requests) == 1 && page.Requests[0].Status == "succeeded" {
			row = page.Requests[0]
			break
		}
	}
	if row.ReferenceCost == nil || row.ReferenceCost.Adjustment == nil || row.ReferenceCost.Adjustment.USD != nil || row.ReferenceCost.Adjustment.Reason != "missing_multiplier" || len(row.ReferenceCost.Adjustment.Parts) != 5 || row.ReferenceCost.USD == nil || *row.ReferenceCost.USD != "0.002240000000000000" {
		t.Fatal(row)
	}
	for _, args := range [][]string{{"logs", "detail", row.ID}, {"logs", "export"}} {
		raw := run(args...)
		var detail logbook.Request
		if err = json.Unmarshal([]byte(raw), &detail); err != nil || detail.ReferenceCost == nil || detail.ReferenceCost.USD == nil || len(detail.Attempts) != 1 || detail.Attempts[0].CostAdjustment == nil || detail.Attempts[0].CostAdjustment.SupplierMultiplier == nil || *detail.Attempts[0].CostAdjustment.SupplierMultiplier != "1" || detail.Attempts[0].ReferenceUSD == nil || strings.Contains(raw, secret) {
			t.Fatal(raw, err)
		}
	}
	var help bytes.Buffer
	if err := (App{Out: &help, Err: io.Discard}).Run(context.Background(), []string{"help"}); err != nil || !strings.Contains(help.String(), "reference_cost") || !strings.Contains(help.String(), "cost_adjustment") {
		t.Fatal(help.String(), err)
	}
}
