package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestNewAPIModelCatalogPaths(t *testing.T) {
	for _, tc := range []struct {
		name, base, directory string
		firstStatus           int
		wantPaths             []string
	}{
		{"root-html", "", "/v1/models", 200, []string{"/models", "/v1/models"}},
		{"prefix-html", "/newapi/", "/newapi/v1/models", 200, []string{"/newapi/models", "/newapi/v1/models"}},
		{"prefix-404", "/gateway/newapi", "/gateway/newapi/v1/models", 404, []string{"/gateway/newapi/models", "/gateway/newapi/v1/models"}},
		{"prefix-405", "/gateway/newapi", "/gateway/newapi/v1/models", 405, []string{"/gateway/newapi/models", "/gateway/newapi/v1/models"}},
		{"explicit-v1", "/newapi/v1/", "/newapi/v1/models", 200, []string{"/newapi/v1/models"}},
		{"explicit-v2", "/newapi/v2", "/newapi/v2/models", 200, []string{"/newapi/v2/models"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			var paths []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer fixture-newapi-token" || r.Header.Get("New-Api-User") != "" || r.URL.RawQuery != "" {
					t.Error("unexpected model catalog request")
				}
				if r.URL.Path != tc.directory {
					// NewAPI's SPA may serve HTML at an unknown catalog path.
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.WriteHeader(tc.firstStatus)
					io.WriteString(w, "<!doctype html><html>NewAPI dashboard</html>")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"success":true,"object":"list","data":[{"id":"model-z","owned_by":"newapi"},{"id":"model-a"},{"id":"model-a"}]}`)
			}))
			defer upstream.Close()
			models, status, failure := f.core.readProviderCatalog(t.Context(), state.Provider{BaseURL: upstream.URL + tc.base, Secret: "fixture-newapi-token", Protocol: "responses"})
			if status != 200 || failure != "" || !reflect.DeepEqual(models, []string{"model-a", "model-z"}) || !reflect.DeepEqual(paths, tc.wantPaths) {
				t.Fatalf("models=%v status=%d failure=%s paths=%v", models, status, failure, paths)
			}
		})
	}
}

func TestPrefixedCatalogDoesNotFallbackOnFailures(t *testing.T) {
	for _, tc := range []struct {
		status  int
		body    string
		failure string
	}{
		{401, "private upstream error", "authentication"},
		{403, "", "authentication"},
		{429, "", "rate_limited"},
		{503, "", "http_error"},
		{302, "", "http_error"},
		{200, `{"data":null}`, "invalid_response"},
	} {
		t.Run(fmt.Sprintf("%d-%s", tc.status, tc.failure), func(t *testing.T) {
			f := setup(t)
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/newapi/models" {
					t.Error("unexpected fallback")
				}
				w.Header().Set("Location", "/newapi/v1/models")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer upstream.Close()
			models, status, failure := f.core.readProviderCatalog(context.Background(), state.Provider{BaseURL: upstream.URL + "/newapi"})
			if status != tc.status || failure != tc.failure || models == nil || len(models) != 0 || calls != 1 {
				t.Fatalf("models=%v status=%d failure=%s calls=%d", models, status, failure, calls)
			}
		})
	}
}

func TestNewAPIModelCatalogSelectedKey(t *testing.T) {
	for _, protocol := range []string{"responses", "chat-completions", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			f := setup(t)
			cfg, _ := f.store.Settings()
			var paths []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.URL.Path == "/newapi/v1/sub2api/billing" {
					if r.Header.Get("Authorization") != "Bearer fixture-restricted-token" || r.Header.Get("x-api-key") != "" {
						t.Error("incorrect billing probe auth")
					}
					w.WriteHeader(404)
					return
				}
				if r.Method != http.MethodGet || r.Header.Get("New-Api-User") != "" {
					t.Error("unexpected catalog request")
				}
				if protocol == "anthropic" {
					if r.Header.Get("x-api-key") != "fixture-restricted-token" || r.Header.Get("anthropic-version") != "2023-06-01" || r.Header.Get("Authorization") != "" {
						t.Error("wrong Anthropic catalog authentication")
					}
				} else if r.Header.Get("Authorization") != "Bearer fixture-restricted-token" || r.Header.Get("x-api-key") != "" {
					t.Error("wrong selected Key")
				}
				if r.URL.Path == "/newapi/models" {
					w.Header().Set("Content-Type", "text/html")
					io.WriteString(w, "<html>NewAPI</html>")
					return
				}
				if r.URL.Path != "/newapi/v1/models" {
					t.Error("incorrect catalog path")
				}
				if protocol == "anthropic" {
					io.WriteString(w, `{"data":[{"id":"allowed-model","display_name":"Allowed"}],"first_id":"allowed-model","last_id":"allowed-model","has_more":false}`)
				} else {
					io.WriteString(w, `{"success":true,"object":"list","data":[{"id":"allowed-model","owned_by":"newapi"}]}`)
				}
			}))
			defer upstream.Close()
			f.add(t, "newapi", upstream.URL+"/newapi", "fixture-default-token")
			if err := f.store.MutateProvider("newapi", 1, "key-put", "restricted", "fixture-restricted-token", "", true, false, protocol); err != nil {
				t.Fatal(err)
			}
			before, _ := f.store.ProviderKeys("newapi")
			status, raw := discover(t, f, "newapi", `{"provider_id":"newapi","expected_revision":2,"key_id":"restricted"}`, f.owner, cfg.InstanceID)
			var result catalog.ProviderModels
			if status != 200 || json.Unmarshal([]byte(raw), &result) != nil || result.Failure != "" || result.HTTPStatus != 200 || result.KeyID != "restricted" || !reflect.DeepEqual(result.Models, []string{"allowed-model"}) || !reflect.DeepEqual(paths, []string{"/newapi/models", "/newapi/v1/models", "/newapi/v1/sub2api/billing"}) {
				t.Fatal(status, raw, paths)
			}
			after, _ := f.store.ProviderKeys("newapi")
			p, _ := f.store.Provider("newapi")
			mappings, _ := f.store.Models()
			if !reflect.DeepEqual(before, after) || p.Revision != 2 || len(mappings) != 0 || strings.Contains(raw, "fixture-restricted-token") {
				t.Fatal("catalog read changed configuration or exposed the credential")
			}
		})
	}
}
