package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func installDefaultsCatalog(t *testing.T, f *fixture) {
	t.Helper()
	raw, err := os.ReadFile("../modeldev/testdata/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	f.core.modelCatalog.Close()
	f.core.modelCatalog = modeldev.NewWithTransport(modelCatalogTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != modeldev.Endpoint || r.Header.Get("Authorization") != "" {
			t.Error("catalog credentials/endpoint")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw))}, nil
	}))
}
func defaultsClient(t *testing.T, f *fixture) *client.Client {
	t.Helper()
	cfg, _ := f.store.Settings()
	cl, _, err := client.Connect(context.Background(), state.Context{Name: "test", Endpoint: f.control.URL, InstanceID: cfg.InstanceID, CredentialRef: "local-owner"}, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cl.Close)
	return cl
}
func TestMappingCreationCatalogDefaults(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1", "synthetic")
	installDefaultsCatalog(t, f)
	cl := defaultsClient(t, f)
	m, err := cl.PutModel(context.Background(), state.Model{ID: "custom-alias", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "vision-reasoner"}}}, 0)
	if err != nil || m.Revision != 1 || m.Settings == nil {
		t.Fatal(m, err)
	}
	cfg := m.Settings.Configuration
	if *cfg.Limits.Context != 250000 || *cfg.Limits.Input != 250000 || *cfg.Limits.Output != 100000 || !*cfg.Capabilities.Vision || len(cfg.SupportedReasoning) != 3 || cfg.DefaultReasoning != nil {
		t.Fatal(cfg)
	}
	if *m.Settings.Reference.Models[0].Pricing.Rates.Input != "2.5" || *m.Settings.Reference.Models[0].Limits.Context != 400000 {
		t.Fatal(m.Settings.Reference)
	}
	// Updates, including old callers with no settings, retain the imported metadata.
	m.Settings = nil
	m, err = cl.PutModel(context.Background(), m, 1)
	if err != nil || m.Settings == nil || *m.Settings.Configuration.Limits.Context != 250000 {
		t.Fatal(m, err)
	}
	prices, _ := f.store.Prices("p")
	if len(prices) != 0 {
		t.Fatal("reference overwrote billing rates", prices)
	}
	for _, tc := range []struct {
		id         string
		candidates []state.Candidate
	}{
		{"missing", []state.Candidate{{ProviderID: "p", Model: "not-listed"}}},
		{"different", []state.Candidate{{ProviderID: "p", Model: "text-only"}, {ProviderID: "p", Model: "vision-reasoner"}}},
	} {
		v, err := cl.PutModel(context.Background(), state.Model{ID: tc.id, Candidates: tc.candidates}, 0)
		if err != nil || v.Settings != nil {
			t.Fatal(tc.id, v, err)
		}
	}
}
func TestProviderScopedCreationCatalogDefaults(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1", "synthetic")
	installDefaultsCatalog(t, f)
	cfg, _ := f.store.Settings()
	req, _ := http.NewRequest("POST", f.control.URL+"/control/v1/providers/p/action", strings.NewReader(`{"action":"model-put","provider_id":"p","expected_revision":1,"target":"alias","model":"vision-reasoner","tools":true,"streaming":true}`))
	req.Header.Set("Authorization", "Bearer "+f.owner)
	req.Header.Set("X-Anchor-Instance", cfg.InstanceID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, raw)
	}
	m, err := f.store.Model("alias")
	if err != nil || m.Settings == nil || *m.Settings.Configuration.Limits.Context != 250000 {
		t.Fatal(m, err)
	}
	// Attaching another provider never replaces shared settings.
	f.add(t, "q", "http://127.0.0.1:1", "synthetic")
	req, _ = http.NewRequest("POST", f.control.URL+"/control/v1/providers/q/action", strings.NewReader(`{"action":"model-put","provider_id":"q","expected_revision":1,"target":"alias","model":"text-only"}`))
	req.Header.Set("Authorization", "Bearer "+f.owner)
	req.Header.Set("X-Anchor-Instance", cfg.InstanceID)
	joined, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer joined.Body.Close()
	body, _ := io.ReadAll(joined.Body)
	if joined.StatusCode != 200 {
		t.Fatalf("%d %s", joined.StatusCode, body)
	}
	m, err = f.store.Model("alias")
	if err != nil || len(m.Candidates) != 2 || m.Revision != 2 || m.Settings == nil || m.Settings.Reference.Models[0].ID != "vision-reasoner" {
		t.Fatal(m, err)
	}
}
func TestExplicitReferenceFillsConfiguration(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1", "synthetic")
	installDefaultsCatalog(t, f)
	cl := defaultsClient(t, f)
	m, err := f.store.PutModel(state.Model{ID: "old-mapping", Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err := cl.ModelSettings(context.Background(), catalog.ModelSettingsQuery{ModelID: m.ID})
	if err != nil {
		t.Fatal(err)
	}
	v, err = cl.SaveModelSettings(context.Background(), catalog.SaveModelSettings{ModelID: m.ID, ExpectedRevision: 1, Configuration: v.Configuration, ReferenceAction: "set", ReferenceProviderID: "lab", ReferenceModelID: "vision-reasoner"})
	if err != nil || v.Reference == nil || *v.Configuration.Limits.Context != 250000 || *v.Reference.Models[0].Pricing.Rates.Input != "2.5" || *v.Configuration.Capabilities.ToolCall {
		t.Fatal(v, err)
	}
}

func TestExistingMappingCatalogDefaultsPreviewAndSave(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1", "synthetic")
	installDefaultsCatalog(t, f)
	cl := defaultsClient(t, f)
	for _, partial := range []bool{false, true} {
		m := state.Model{ID: "legacy", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "vision-reasoner"}}}
		if partial {
			m.ID = "partial"
			m.Settings = &state.ModelSettings{}
			contextLimit, outputLimit := int64(500000), int64(64000)
			vision := false
			m.Settings.Configuration.Limits.Context = &contextLimit
			m.Settings.Configuration.Limits.Output = &outputLimit
			m.Settings.Configuration.Capabilities.Vision = &vision
		}
		original, err := f.store.PutModel(m, 0)
		if err != nil {
			t.Fatal(err)
		}
		v, err := cl.ModelSettings(t.Context(), catalog.ModelSettingsQuery{ModelID: m.ID})
		if err != nil || v.DefaultsStatus != "matched" || v.Reference == nil || v.Configuration.Limits.Output == nil || v.ContextPolicy == nil || *v.ContextPolicy.PriceThreshold != 250000 || *v.ContextPolicy.ModelContext != 400000 {
			t.Fatal(v, err)
		}
		if partial {
			if *v.Configuration.Limits.Context != 500000 || *v.Configuration.Limits.Output != 64000 || *v.Configuration.Capabilities.Vision {
				t.Fatal("manual overrides replaced", v)
			}
		} else if *v.Configuration.Limits.Context != 250000 || *v.Configuration.Limits.Output != 100000 {
			t.Fatal("missing catalog defaults", v)
		}
		after, _ := f.store.Model(m.ID)
		if !reflect.DeepEqual(original, after) {
			t.Fatal("GET mutated stored model", original, after)
		}
		source := v.Reference.Models[0]
		saved, err := cl.SaveModelSettings(t.Context(), catalog.SaveModelSettings{ModelID: m.ID, ExpectedRevision: v.Revision, Configuration: v.Configuration, ReferenceAction: "set", ReferenceProviderID: source.ProviderID, ReferenceModelID: source.ID})
		if err != nil || saved.Revision != 2 || saved.DefaultsStatus != "saved" || saved.Reference == nil {
			t.Fatal(saved, err)
		}
		prices, _ := f.store.Prices("p")
		if len(prices) != 0 {
			t.Fatal("reference copied into billing", prices)
		}
	}
}

func TestExistingMappingCatalogDefaultsFallbacks(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1", "synthetic")
	installDefaultsCatalog(t, f)
	cl := defaultsClient(t, f)
	for _, tc := range []struct {
		id, upstream, tier, status string
		extra                      bool
	}{
		{"missing-old", "absent", "", "unmatched", false},
		{"different-old", "vision-reasoner", "", "unmatched", true},
		{"fast-old", "vision-reasoner", "priority", "matched", false},
	} {
		m := state.Model{ID: tc.id, ServiceTier: tc.tier, Candidates: []state.Candidate{{ProviderID: "p", Model: tc.upstream}}}
		if tc.extra {
			m.Candidates = append(m.Candidates, state.Candidate{ProviderID: "p", Model: "text-only"})
		}
		if _, err := f.store.PutModel(m, 0); err != nil {
			t.Fatal(err)
		}
		v, err := cl.ModelSettings(t.Context(), catalog.ModelSettingsQuery{ModelID: m.ID})
		if err != nil || v.DefaultsStatus != tc.status || v.Reference != nil {
			t.Fatal(tc.id, v, err)
		}
		if tc.tier != "" && (v.Configuration.Limits.Context == nil || *v.Configuration.Limits.Context != 250000 || v.ContextPolicy == nil || *v.ContextPolicy.PriceThreshold != 250000) {
			t.Fatal(v)
		}
	}
}

func TestExistingMappingCatalogDefaultsPinnedAndUnavailable(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1", "synthetic")
	installDefaultsCatalog(t, f)
	snapshot, err := f.core.modelCatalog.Resolve(t.Context(), "vision-reasoner")
	if err != nil {
		t.Fatal(err)
	}
	f.core.modelCatalog.Close()
	calls := 0
	f.core.modelCatalog = modeldev.NewWithTransport(modelCatalogTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("synthetic offline")
	}))
	cl := defaultsClient(t, f)
	m := state.Model{ID: "pinned", Candidates: []state.Candidate{{ProviderID: "p", Model: "not-listed"}}, Settings: &state.ModelSettings{Reference: &snapshot}}
	if _, err = f.store.PutModel(m, 0); err != nil {
		t.Fatal(err)
	}
	v, err := cl.ModelSettings(t.Context(), catalog.ModelSettingsQuery{ModelID: m.ID})
	if err != nil || v.DefaultsStatus != "reference" || v.Configuration.Limits.Context == nil || calls != 0 {
		t.Fatal(v, err, calls)
	}
	m.ID, m.Settings = "offline", nil
	if _, err = f.store.PutModel(m, 0); err != nil {
		t.Fatal(err)
	}
	v, err = cl.ModelSettings(t.Context(), catalog.ModelSettingsQuery{ModelID: m.ID})
	if err != nil || v.DefaultsStatus != "unavailable" || v.Configuration.Limits.Context != nil || calls != 1 {
		t.Fatal(v, err, calls)
	}
}
