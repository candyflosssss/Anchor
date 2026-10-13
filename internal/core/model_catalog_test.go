package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/modeldev"
)

type modelCatalogTransport func(*http.Request) (*http.Response, error)

func (f modelCatalogTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestModelCatalogControlBoundariesAndPriceImport(t *testing.T) {
	f := setup(t)
	raw, e := os.ReadFile("../modeldev/testdata/catalog.json")
	if e != nil {
		t.Fatal(e)
	}
	var calls atomic.Int32
	f.core.modelCatalog.Close()
	f.core.modelCatalog = modeldev.NewWithTransport(modelCatalogTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.String() != modeldev.Endpoint || r.Header.Get("Authorization") != "" {
			t.Error("fetch boundary")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
	}))
	cfg, _ := f.store.Settings()
	for _, tc := range []struct {
		token, identity, origin, body string
		status                        int
	}{
		{"", cfg.InstanceID, "", `{}`, 401},
		{f.token, cfg.InstanceID, "", `{}`, 401},
		{f.owner, "wrong-instance", "", `{}`, 409},
		{f.owner, cfg.InstanceID, "https://example.invalid", `{}`, 403},
		{f.owner, cfg.InstanceID, "", `{"model_id":"m"}`, 400},
		{f.owner, cfg.InstanceID, "", `{"provider_id":"missing","model_id":"m"}`, 404},
		{f.owner, cfg.InstanceID, "", `{"provider_id":"lab","model_id":"vision-reasoner"}`, 200},
	} {
		req, _ := http.NewRequest("POST", f.control.URL+"/control/v1/model-catalog/query", strings.NewReader(tc.body))
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		req.Header.Set("X-Anchor-Instance", tc.identity)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("status %d expected %d: %s", resp.StatusCode, tc.status, body)
		}
		if resp.StatusCode == 200 {
			var out modeldev.ModelCatalog
			if json.Unmarshal(body, &out) != nil || len(out.Models) != 1 || !*out.Models[0].Capabilities.Vision {
				t.Fatal(string(body))
			}
		}
	}
	if calls.Load() != 1 {
		t.Fatal("cache/boundary fetch count", calls.Load())
	}
	p, e := f.core.importPrice(context.Background(), "local-alias", "lab", "vision-reasoner")
	if e != nil || p.Model != "local-alias" || p.InputUSD != 2.5 || p.Source != "models.dev/lab/vision-reasoner" || calls.Load() != 1 {
		t.Fatal(p, e, calls.Load())
	}
	if _, e := f.core.importPrice(context.Background(), "alias", "", ""); e == nil {
		t.Fatal("empty catalog ID accepted")
	}
}
