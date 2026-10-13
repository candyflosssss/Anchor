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

func TestCatalogMergesAllKeysAndFetchesRates(t *testing.T) {
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
	if err := f.store.MutateProvider("p", 1, "key-put", "second", "catalog-second-fixture", "", false, false, "anthropic"); err != nil {
		t.Fatal(err)
	}
	if err := f.store.MutateProvider("p", 2, "key-put", "third", "catalog-third-fixture", "", true, false, "responses"); err != nil {
		t.Fatal(err)
	}
	token := "account-catalog-fixture"
	if _, err := f.store.ConfigureProviderPlatform(providerapi.Configure{ProviderID: "p", ExpectedRevision: 3, Config: providerapi.Config{Platform: "sub2api"}, UserToken: &token}); err != nil {
		t.Fatal(err)
	}
	before, _ := f.store.Provider("p")
	beforeKeys, _ := f.store.ProviderKeys("p")
	status, raw := discover(t, f, "p", `{"provider_id":"p","expected_revision":4}`, f.owner, cfg.InstanceID)
	var out catalog.ProviderModels
	if status != 200 || json.Unmarshal([]byte(raw), &out) != nil {
		t.Fatal(status, raw)
	}
	if out.Failure != "" || !out.Partial || out.KeyID != "" || !reflect.DeepEqual(out.Models, []string{"first", "second", "shared"}) || len(out.Keys) != 3 {
		t.Fatal(raw)
	}
	if !reflect.DeepEqual(out.Entries[2].KeyIDs, []string{"default", "second"}) || out.Keys[1].Enabled || out.Keys[2].Failure != "authentication" {
		t.Fatal(raw)
	}
	if out.Keys[0].Multiplier == nil || *out.Keys[0].Multiplier != "0.8" || out.Keys[1].Multiplier == nil || *out.Keys[1].Multiplier != "1.5" || out.Keys[1].GroupName != "Premium" || out.Keys[2].RateFailure != "key_not_found" {
		t.Fatal(raw)
	}
	for _, k := range beforeKeys {
		if calls["Bearer "+k.Secret] != 1 || strings.Contains(raw, k.Secret) {
			t.Fatal("key skipped/retried or leaked", raw)
		}
	}
	after, _ := f.store.Provider("p")
	afterKeys, _ := f.store.ProviderKeys("p")
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(beforeKeys, afterKeys) || strings.Contains(raw, token) {
		t.Fatal("configuration mutated or token leaked")
	}
	status, raw = discover(t, f, "p", `{"provider_id":"p","expected_revision":4,"key_id":"second"}`, f.owner, cfg.InstanceID)
	if status != 200 || json.Unmarshal([]byte(raw), &out) != nil || out.KeyID != "second" || out.Partial || len(out.Keys) != 1 || len(out.Models) != 2 {
		t.Fatal(raw)
	}
}
