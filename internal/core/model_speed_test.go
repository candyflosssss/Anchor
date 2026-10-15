package core

import (
	"encoding/json"
	"fmt"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestMappingSpeedInjection(t *testing.T) {
	for _, tier := range []string{"priority", "ultrafast"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", tier, stream), func(t *testing.T) {
				f := setup(t)
				calls := 0
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["model"] != "gpt-6.1-sol" || body["service_tier"] != tier {
						t.Errorf("wrong effective request: %v", body)
					}
					// A downgrade must not be presented as a successful Fast acknowledgement.
					response := `{"id":"r","status":"completed","service_tier":"default","output":[],"usage":{"input_tokens":1,"output_tokens":0}}`
					w.Header().Set("Content-Type", "application/json")
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						response = "data: {\"type\":\"response.completed\",\"response\":" + response + "}\n\n"
					}
					io.WriteString(w, response)
				}))
				defer up.Close()
				f.add(t, "p", up.URL, "synthetic")
				speed := "fast"
				if tier == "ultrafast" {
					speed = "ultrafast"
				}
				id, _, _ := state.SpeedVariantID("gpt-6.1-sol", speed)
				_, err := f.store.PutModel(state.Model{ID: id, ServiceTier: tier, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "gpt-6.1-sol"}}}, 0)
				if err != nil {
					t.Fatal(err)
				}
				for _, caller := range []string{"", `,"service_tier":"default"`} {
					resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, fmt.Sprintf(`{"model":%q,"input":"hi","stream":%t%s}`, id, stream, caller))
					text := text(t, resp)
					if resp.StatusCode != 200 {
						t.Fatal(resp.StatusCode, text)
					}
					if err := f.core.logs.Flush(t.Context()); err != nil {
						t.Fatal(err)
					}
					row, err := logClient(t, f).LogDetail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
					wantCaller := "unspecified"
					if caller != "" {
						wantCaller = "default"
					}
					if err != nil || row.Evidence["mapping_service_tier"] != tier || row.Evidence["requested_service_tier"] != wantCaller || row.Evidence["upstream_service_tier"] != "default" {
						t.Fatalf("%+v %v", row, err)
					}
				}
				if calls != 2 {
					t.Fatal(calls)
				}
			})
		}
	}
}

func TestProviderSpeedVariantAction(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "https://example.invalid", "synthetic")
	cl := logClient(t, f)
	raw, err := os.ReadFile("../../desktop/contract/model-speed-variants.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Action   catalog.Action         `json:"action"`
		Variants []catalog.ModelMapping `json:"variants"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	in := fixture.Action
	w, err := cl.ProviderAction(t.Context(), in)
	if err != nil || len(w.Models) != 3 {
		t.Fatal(w, err)
	}
	for _, expected := range fixture.Variants {
		found := false
		for _, m := range w.Models {
			if m.ID == expected.ID {
				found = true
				if m.ServiceTier != expected.ServiceTier || m.Upstream != expected.Upstream {
					t.Fatal(m, expected)
				}
			}
		}
		if !found {
			t.Fatal("missing variant", expected.ID)
		}
	}
	// All management operations continue to accept the exact parenthesized ID.
	id := "gpt-6.1-sol(fast)"
	settings, err := cl.ModelSettings(t.Context(), catalog.ModelSettingsQuery{ModelID: id})
	if err != nil || settings.ModelID != id {
		t.Fatal(settings, err)
	}
	tier := "ultrafast"
	edit := catalog.Action{ProviderID: "p", ExpectedRevision: 1, Action: "model-put", Target: id, Model: "gpt-6.1-sol", ModelRevision: 1, ServiceTier: &tier, Tools: true, Streaming: true}
	if _, err := cl.ProviderAction(t.Context(), edit); err != nil {
		t.Fatal(err)
	}
	edit.ModelRevision = 2
	edit.ServiceTier = nil
	if _, err := cl.ProviderAction(t.Context(), edit); err != nil {
		t.Fatal(err)
	}
	saved, _ := f.store.Model(id)
	if saved.ServiceTier != tier {
		t.Fatal("omitted field cleared tier", saved)
	}
	in.Target = "another"
	in.SpeedVariants = []string{"fast", "fast"}
	if _, err := cl.ProviderAction(t.Context(), in); err == nil {
		t.Fatal("invalid speeds accepted")
	}
	if _, err := f.store.Model("another"); err == nil {
		t.Fatal("invalid action wrote base")
	}
}

func TestSpeedVariantsKeepCapabilitiesWithoutStandardPrices(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1", "synthetic")
	installDefaultsCatalog(t, f)
	cl := defaultsClient(t, f)
	_, err := cl.ProviderAction(t.Context(), catalog.Action{ProviderID: "p", ExpectedRevision: 1, Action: "model-put", Target: "priced", Model: "vision-reasoner", Tools: true, Streaming: true, SpeedVariants: []string{"fast", "ultrafast"}})
	if err != nil {
		t.Fatal(err)
	}
	base, err := f.store.Model("priced")
	if err != nil || base.Settings == nil || base.Settings.Reference == nil {
		t.Fatal(base, err)
	}
	for _, speed := range []string{"fast", "ultrafast"} {
		m, err := f.store.Model("priced(" + speed + ")")
		if err != nil || m.Settings == nil || m.Settings.Reference != nil || m.Settings.Configuration.Limits.Context == nil || *m.Settings.Configuration.Limits.Context != 400000 {
			t.Fatal(m, err)
		}
	}
	// A directly created fixed-tier model must not capture Standard reference rates either.
	fixed, err := cl.PutModel(t.Context(), state.Model{ID: "fixed", ServiceTier: "priority", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "vision-reasoner"}}}, 0)
	if err != nil || fixed.Settings == nil || fixed.Settings.Reference != nil || fixed.Settings.Configuration.Limits.Context == nil {
		t.Fatal(fixed, err)
	}
}
