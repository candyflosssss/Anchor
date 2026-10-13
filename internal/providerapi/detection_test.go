package providerapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAutomaticPlatformAndDefaultKeySummary(t *testing.T) {
	for _, tc := range []struct{ name, platform, status, settings, usage, token, scope, value, fail string }{
		{name: "wallet", platform: "sub2api", settings: `{"code":0,"data":{"registration_enabled":true,"site_name":"Custom","version":"1","api_base_url":""}}`, usage: `{"mode":"unrestricted","balance":12.34567890123456789}`, scope: "account", value: "12.34567890123456789"},
		{name: "key cap", platform: "sub2api", usage: `{"mode":"quota_limited","quota":{"remaining":2.34,"used":1,"limit":3.34}}`, scope: "key", value: "2.34"},
		{name: "newapi", platform: "newapi", status: `{"success":true,"data":{"version":"1","quota_per_unit":500000,"default_use_auto_group":true,"enable_batch_update":false}}`, token: `{"code":true,"data":{"total_available":9007199254740993123,"total_used":2,"total_granted":9007199254740993125}}`, scope: "key", value: "9007199254740993123"},
		{name: "key endpoint fallback", platform: "newapi", token: `{"code":true,"data":{"total_available":0,"total_used":0,"total_granted":0}}`, scope: "key", value: "0"},
		{name: "oneapi", platform: "oneapi", status: `{"success":true,"data":{"version":"1","quota_per_unit":500000,"oidc":false,"oidc_well_known":""}}`, fail: "auth_required"},
		{name: "unknown", status: `{"success":true,"data":{"system_name":"NewAPI","version":"1","quota_per_unit":500000}}`, fail: "platform_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if !strings.HasPrefix(r.URL.Path, "/prefix/") {
					t.Fatal("lost deployment prefix")
				}
				path := strings.TrimPrefix(r.URL.Path, "/prefix")
				body := ""
				switch path {
				case "/api/status":
					body = tc.status
				case "/api/v1/settings/public":
					body = tc.settings
				case "/v1/usage":
					body = tc.usage
				case "/v1/dashboard/billing/subscription":
					w.WriteHeader(404)
					return
				case "/api/usage/token/":
					body = tc.token
				default:
					t.Error("unexpected path", path)
				}
				want := "Bearer default-key-fixture"
				if path == "/api/status" || path == "/api/v1/settings/public" {
					want = ""
				}
				if r.Header.Get("Authorization") != want {
					t.Error("wrong credential", path)
				}
				if r.Header.Get("New-Api-User") != "" {
					t.Error("summary used user authorization")
				}
				if body == "" {
					w.WriteHeader(404)
					return
				}
				io.WriteString(w, body)
			}))
			defer upstream.Close()
			a := Adapter{HTTP: upstream.Client(), BaseURL: upstream.URL + "/prefix/v1", Key: "default-key-fixture", Config: Config{Platform: "auto", UserToken: "user-fixture", UserID: 7}}
			result := a.Query(context.Background(), Query{ProviderID: "demo", ExpectedRevision: 1, Operation: "balance-summary"})
			if result.Platform != tc.platform || result.Failure != tc.fail {
				t.Fatalf("%+v", result)
			}
			if tc.fail == "" && (result.Balance == nil || result.Balance.Scope != tc.scope || result.Balance.Remaining.Value.String() != tc.value) {
				t.Fatalf("%+v", result.Balance)
			}
			if requests > 8 {
				t.Fatal("unbounded detection")
			}
		})
	}
}

func TestDetectionFailuresAndNoRedirects(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, fail string
	}{
		{200, "<html>NewAPI</html>", "platform_unknown"}, {401, "", "authentication"}, {403, "", "permission_denied"}, {429, "", "rate_limited"}, {302, "", "platform_unknown"},
	} {
		t.Run(tc.fail, func(t *testing.T) {
			redirected := false
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirected" {
					redirected = true
				}
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer upstream.Close()
			a := Adapter{HTTP: upstream.Client(), BaseURL: upstream.URL + "/v1", Key: "fixture"}
			r := a.Query(context.Background(), Query{ExpectedRevision: 1, Operation: "detect-platform"})
			if r.Failure != tc.fail || redirected {
				t.Fatalf("%+v redirect=%v", r, redirected)
			}
		})
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer upstream.Close()
	a := Adapter{HTTP: upstream.Client(), BaseURL: upstream.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if r := a.Query(ctx, Query{ExpectedRevision: 1, Operation: "detect-platform"}); r.Failure != "timeout" {
		t.Fatal(r)
	}
}
