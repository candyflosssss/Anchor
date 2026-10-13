package providerapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func billingFixture(multiplier string) string {
	return `{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":2,"user_rate_multiplier":0.8,"resolved_rate_multiplier":0.8,"peak_rate_enabled":true,"applied_peak_multiplier":1.5,"effective_rate_multiplier":` + multiplier + `}`
}

func TestSub2KeyBillingWithoutUserAuthorization(t *testing.T) {
	for _, platform := range []string{"sub2api", "auto", ""} {
		for _, rate := range []string{"1.2", "0", "0.8123456789012345"} {
			t.Run(platform+"/"+rate, func(t *testing.T) {
				billingCalls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/prefix/v1/sub2api/billing" {
						if r.Header.Get("Authorization") != "" {
							t.Error("credentials sent to public probe")
						}
						w.WriteHeader(404)
						return
					}
					billingCalls++
					if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer inference-billing-fixture" || r.URL.RawQuery != "" || r.Header.Get("New-Api-User") != "" {
						t.Error("incorrect key request")
					}
					fmt.Fprint(w, billingFixture(rate))
				}))
				defer server.Close()
				a := Adapter{HTTP: server.Client(), BaseURL: server.URL + "/prefix/v1", Config: Config{Platform: platform}, Key: "inference-billing-fixture"}
				out := a.QueryKeyRate(context.Background())
				if out.Failure != "" || out.Platform != "sub2api" || out.EffectiveRate == nil || out.EffectiveRate.Multiplier.String() != rate || out.EffectiveRate.Basis != "key_billing" || out.EffectiveRate.Group != "" || out.EffectiveRate.GroupName != "" || billingCalls != 1 {
					t.Fatalf("%+v calls=%d", out, billingCalls)
				}
				raw, _ := json.Marshal(out)
				if strings.Contains(string(raw), a.Key) {
					t.Fatal("secret leaked")
				}
			})
		}
	}
}

func TestSub2KeyBillingFailuresDoNotUseUserAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		body, failure string
	}{
		{"unauthorized", 401, "secret", "authentication"}, {"forbidden", 403, "secret", "permission_denied"},
		{"limited", 429, "secret", "rate_limited"}, {"server", 500, "secret", "http_error"},
		{"wrong-schema", 200, `{}`, "invalid_response"}, {"bad-json", 200, `{`, "invalid_response"},
		{"missing-rate", 200, strings.Replace(billingFixture("1.2"), `"effective_rate_multiplier":1.2`, `"unused":1.2`, 1), "invalid_response"},
		{"negative", 200, billingFixture("-1"), "invalid_response"}, {"overflow", 200, billingFixture("1e309"), "invalid_response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != sub2KeyBillingPath || r.Header.Get("Authorization") != "Bearer inference-billing-fixture" {
					t.Error("unexpected fallback or user credential")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			a := Adapter{HTTP: server.Client(), BaseURL: server.URL, Key: "inference-billing-fixture", Config: Config{Platform: "sub2api", UserToken: "user-billing-fixture"}}
			out := a.QueryKeyRate(context.Background())
			if out.Failure != tc.failure || out.EffectiveRate != nil || calls != 1 {
				t.Fatal(out, calls)
			}
		})
	}
}

func TestSub2AbsentBillingFallsBackToLegacy(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{404, ""}, {405, ""}, {200, "<!doctype html><html>Legacy dashboard</html>"}} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path == sub2KeyBillingPath {
					if r.Header.Get("Authorization") != "Bearer inference-billing-fixture" {
						t.Error("incorrect probe auth")
					}
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
					return
				}
				if r.Header.Get("Authorization") != "Bearer user-billing-fixture" {
					t.Error("incorrect legacy auth")
				}
				switch r.URL.Path {
				case "/api/v1/keys":
					fmt.Fprint(w, `{"data":{"items":[{"id":7,"key":"inference-billing-fixture"}],"total":1}}`)
				case "/api/v1/keys/7":
					fmt.Fprint(w, `{"data":{"group_id":3}}`)
				case "/api/v1/groups/rates":
					fmt.Fprint(w, `{"data":{"3":0.8}}`)
				default:
					t.Error("unexpected request", r.URL.Path)
				}
			}))
			defer server.Close()
			a := Adapter{HTTP: server.Client(), BaseURL: server.URL, Key: "inference-billing-fixture", Config: Config{Platform: "sub2api", UserToken: "user-billing-fixture"}}
			out := a.QueryKeyRate(context.Background())
			if out.Failure != "" || out.EffectiveRate == nil || out.EffectiveRate.Multiplier.String() != "0.8" || out.EffectiveRate.Basis != "personal_override" || calls != 4 {
				t.Fatal(out, calls)
			}
		})
	}
}

func TestSub2ExplicitRemoteIDDoesNotUseSavedKeyBilling(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Error("missing user authorization must not query another key")
	}))
	defer server.Close()
	a := Adapter{HTTP: server.Client(), BaseURL: server.URL, Key: "inference-billing-fixture", Config: Config{Platform: "sub2api"}}
	out := a.Query(context.Background(), Query{Operation: "effective-rate", ExpectedRevision: 1, RemoteKeyID: 7})
	if out.Failure != "auth_required" || calls != 0 {
		t.Fatal(out, calls)
	}
}
