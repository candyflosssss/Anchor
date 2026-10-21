package core

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/providerapi"
)

func TestCatalogMergesEnabledKeysAndFetchesRates(t *testing.T) {
	f := setup(t)
	cfg, _ := f.store.Settings()
	calls := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			auth := r.Header.Get("Authorization")
			if key := r.Header.Get("x-api-key"); key != "" {
				auth = "Bearer " + key
			}
			calls[auth]++
			switch auth {
			case "Bearer catalog-first-fixture":
				io.WriteString(w, `{"data":[{"id":"shared"},{"id":"first"}]}`)
			case "Bearer catalog-second-fixture":
				if r.Header.Get("x-api-key") != "catalog-second-fixture" {
					t.Error("missing anthropic authorization")
				}
				io.WriteString(w, `{"data":[{"id":"shared"},{"id":"second"}]}`)
			default:
				w.WriteHeader(401)
			}
			return
		}
		calls[r.URL.Path]++
		if r.URL.Path == "/v1/sub2api/billing" {
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer catalog-") {
				t.Error("incorrect key billing auth")
			}
			w.WriteHeader(404)
			return
		}
		if r.Header.Get("Authorization") != "Bearer account-catalog-fixture" {
			t.Error("incorrect billing auth")
		}
		switch r.URL.Path {
		case "/api/v1/keys":
			io.WriteString(w, `{"code":0,"data":{"items":[{"id":11,"key":"catalog-first-fixture"},{"id":12,"key":"catalog-second-fixture"}],"total":2}}`)
		case "/api/v1/keys/11":
			io.WriteString(w, `{"code":0,"data":{"group_id":1,"group":{"name":"Standard"}}}`)
		case "/api/v1/keys/12":
			io.WriteString(w, `{"code":0,"data":{"group_id":2,"group":{"name":"Premium"}}}`)
		case "/api/v1/groups/rates":
			io.WriteString(w, `{"code":0,"data":{"1":0.8,"2":1.5}}`)
		default:
			t.Error("unexpected path", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	f.add(t, "p", upstream.URL+"/v1", "catalog-first-fixture")
	if err := f.store.MutateProvider("p", 1, "key-put", "second", "catalog-second-fixture", "", true, false, "anthropic"); err != nil {
		t.Fatal(err)
	}
	if err := f.store.MutateProvider("p", 2, "key-put", "third", "catalog-third-fixture", "", true, false, "responses"); err != nil {
		t.Fatal(err)
	}
	if err := f.store.MutateProvider("p", 3, "key-put", "disabled", "catalog-disabled-fixture", "", false, false, "responses"); err != nil {
		t.Fatal(err)
	}
	token := "account-catalog-fixture"
	if _, err := f.store.ConfigureProviderPlatform(providerapi.Configure{ProviderID: "p", ExpectedRevision: 4, Config: providerapi.Config{Platform: "sub2api"}, UserToken: &token}); err != nil {
		t.Fatal(err)
	}
	before, _ := f.store.Provider("p")
	beforeKeys, _ := f.store.ProviderKeys("p")
	status, raw := discover(t, f, "p", `{"provider_id":"p","expected_revision":5}`, f.owner, cfg.InstanceID)
	var out catalog.ProviderModels
	if status != 200 || json.Unmarshal([]byte(raw), &out) != nil {
		t.Fatal(status, raw)
	}
	if out.Failure != "" || !out.Partial || out.KeyID != "" || !reflect.DeepEqual(out.Models, []string{"first", "second", "shared"}) || len(out.Keys) != 3 {
		t.Fatal(raw)
	}
	if !reflect.DeepEqual(out.Entries[2].KeyIDs, []string{"default", "second"}) || !out.Keys[1].Enabled || out.Keys[2].Failure != "authentication" {
		t.Fatal(raw)
	}
	if out.Keys[0].Multiplier == nil || *out.Keys[0].Multiplier != "0.8" || out.Keys[1].Multiplier == nil || *out.Keys[1].Multiplier != "1.5" || out.Keys[1].GroupName != "Premium" || out.Keys[2].RateFailure != "key_not_found" {
		t.Fatal(raw)
	}
	if calls["/api/v1/keys"] != 3 || calls["/api/v1/keys/11"] != 1 || calls["/api/v1/keys/12"] != 1 || calls["/api/v1/groups/rates"] != 2 {
		t.Fatal("unexpected rate queries", calls)
	}
	for _, k := range beforeKeys {
		wantCalls := 0
		if k.Enabled {
			wantCalls = 1
		}
		if calls["Bearer "+k.Secret] != wantCalls || strings.Contains(raw, k.Secret) {
			t.Fatal("key skipped/retried or leaked", raw)
		}
	}
	after, _ := f.store.Provider("p")
	afterKeys, _ := f.store.ProviderKeys("p")
	for i := range afterKeys {
		k := afterKeys[i]
		switch k.ID {
		case "default", "second":
			want := 0.8
			if k.ID == "second" {
				want = 1.5
			}
			if k.Rate == nil || k.Rate.Multiplier == nil || *k.Rate.Multiplier != want || k.Rate.ObservedAt == "" || k.Rate.Failure != "" {
				t.Fatalf("missing per-Key cache for %s: %+v", k.ID, k.Rate)
			}
		case "third":
			if k.Rate == nil || k.Rate.Failure != "key_not_found" || k.Rate.Multiplier != nil {
				t.Fatalf("unexpected failed cache: %+v", k.Rate)
			}
		case "disabled":
			if k.Rate != nil {
				t.Fatal("queried disabled Key")
			}
		}
		afterKeys[i].Rate = nil
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(beforeKeys, afterKeys) || strings.Contains(raw, token) {
		t.Fatal("configuration mutated or token leaked")
	}
	status, raw = discover(t, f, "p", `{"provider_id":"p","expected_revision":5,"key_id":"second"}`, f.owner, cfg.InstanceID)
	if status != 200 || json.Unmarshal([]byte(raw), &out) != nil || out.KeyID != "second" || out.Partial || len(out.Keys) != 1 || len(out.Models) != 2 {
		t.Fatal(raw)
	}
}

func TestCatalogSkipsDisabledKeysWithoutAnonymousFallback(t *testing.T) {
	for _, tc := range []struct {
		name, keyID, secret string
		disabled            bool
		wantStatus          int
		wantFailure         string
		wantCalls           int
	}{
		{"all disabled", "", "catalog-disabled-fixture", true, 200, "no_enabled_keys", 0},
		{"explicit disabled", "default", "catalog-disabled-fixture", true, 200, "no_enabled_keys", 0},
		{"unknown key", "missing", "catalog-disabled-fixture", true, 404, "", 0},
		{"anonymous cannot bypass disabled", "anonymous", "catalog-disabled-fixture", true, 404, "", 0},
		{"no saved keys", "", "", false, 200, "", 1},
		{"explicit anonymous", "anonymous", "", false, 200, "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			cfg, _ := f.store.Settings()
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "" {
					t.Error("unexpected catalog or rate request", r.URL.Path)
				}
				io.WriteString(w, `{"data":[{"id":"anonymous-model"}]}`)
			}))
			defer upstream.Close()
			f.add(t, "p", upstream.URL+"/v1", tc.secret)
			savedCatalogPlatform(t, f, "p")
			revision := int64(1)
			if tc.disabled {
				if err := f.store.MutateProvider("p", revision, "key-put", "default", "", "", false, false); err != nil {
					t.Fatal(err)
				}
				revision++
				// Rate fetching is configured to catch accidental billing reads, too.
				token := "account-catalog-fixture"
				if _, err := f.store.ConfigureProviderPlatform(providerapi.Configure{ProviderID: "p", ExpectedRevision: revision, Config: providerapi.Config{Platform: "sub2api"}, UserToken: &token}); err != nil {
					t.Fatal(err)
				}
				revision++
			}
			body, _ := json.Marshal(catalog.DiscoverProviderModels{ProviderID: "p", ExpectedRevision: revision, KeyID: tc.keyID})
			status, raw := discover(t, f, "p", string(body), f.owner, cfg.InstanceID)
			if status != tc.wantStatus || calls != tc.wantCalls {
				t.Fatal(status, calls, raw)
			}
			if status != 200 {
				return
			}
			var out catalog.ProviderModels
			if json.Unmarshal([]byte(raw), &out) != nil || out.Failure != tc.wantFailure || out.Partial {
				t.Fatal(raw)
			}
			if tc.disabled {
				if out.KeyID != tc.keyID || len(out.Keys) != 0 || len(out.Models) != 0 || len(out.Entries) != 0 || out.HTTPStatus != 0 {
					t.Fatal("disabled keys were queried", raw)
				}
			} else if out.KeyID != "anonymous" || len(out.Keys) != 1 || !reflect.DeepEqual(out.Entries[0].KeyIDs, []string{"anonymous"}) {
				t.Fatal("anonymous discovery changed", raw)
			}
		})
	}
}
