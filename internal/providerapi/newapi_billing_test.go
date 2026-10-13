package providerapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestNewAPIBillingBalancesWithInferenceKey(t *testing.T) {
	for _, tc := range []struct{ name, total, used, display, unit, remaining string }{
		{"dollars", "20.1234567890123456789", "123.4567890123456789", "USD", "USD", "18.8888888988888888899"},
		{"yuan", "70.5", "50", "CNY", "CNY", "70"},
		{"quota", "9007199254740993125", "200", "TOKENS", "quota", "9007199254740993123"},
		{"unknown currency", "12.3", "100", "", "site_unit", "11.3"},
		{"zero", "0", "0", "USD", "USD", "0"},
		{"debt", "1", "150", "USD", "USD", "-0.5"},
		{"exponents", "1.2e2", "2.3e2", "USD", "USD", "117.7"},
	} {
		for _, op := range []string{"balance", "balance-summary"} {
			t.Run(tc.name+"/"+op, func(t *testing.T) {
				var paths []string
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					path := strings.TrimPrefix(r.URL.Path, "/proxy")
					paths = append(paths, path)
					if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, "/proxy/") || r.Header.Get("New-Api-User") != "" {
						t.Error("wrong request", r.URL.Path)
					}
					if path == "/api/status" {
						if r.Header.Get("Authorization") != "" {
							t.Error("credential in public request")
						}
						fmt.Fprintf(w, `{"success":true,"data":{"quota_display_type":%q}}`, tc.display)
						return
					}
					if r.Header.Get("Authorization") != "Bearer billing-fixture-key" {
						t.Error("wrong API key")
					}
					switch path {
					case "/v1/dashboard/billing/subscription":
						fmt.Fprintf(w, `{"object":"billing_subscription","hard_limit_usd":%s}`, tc.total)
					case "/v1/dashboard/billing/usage":
						fmt.Fprintf(w, `{"object":"list","total_usage":%s}`, tc.used)
					default:
						t.Error("unexpected path", path)
						w.WriteHeader(404)
					}
				}))
				defer up.Close()
				a := Adapter{HTTP: up.Client(), Config: Config{Platform: "newapi"}, BaseURL: up.URL + "/proxy/v1", Key: "billing-fixture-key"}
				v := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: op})
				if v.Failure != "" || v.Balance == nil || v.Balance.Scope != "billing" || v.Balance.Remaining.Value.String() != tc.remaining || v.Balance.Remaining.Unit != tc.unit || v.Balance.Unlimited != nil {
					t.Fatalf("%+v balance=%+v", v, v.Balance)
				}
				if !reflect.DeepEqual(paths, []string{"/v1/dashboard/billing/subscription", "/v1/dashboard/billing/usage", "/api/status"}) {
					t.Fatal(paths)
				}
			})
		}
	}
}

func TestNewAPIBillingFallbackAndFailures(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, fail string
		fallback   bool
	}{
		{404, "", "", true}, {405, "", "", true}, {200, "<html>login</html>", "", true},
		{401, "", "authentication", false}, {403, "", "permission_denied", false},
		{429, "", "rate_limited", false}, {500, "", "http_error", false},
		{200, `{"error":{"message":"rejected"}}`, "upstream_rejected", false},
	} {
		t.Run(fmt.Sprintf("%d/%s", tc.status, tc.body), func(t *testing.T) {
			calls := 0
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("wrong credential")
				}
				if r.URL.Path == "/api/usage/token/" {
					fmt.Fprint(w, `{"code":true,"data":{"total_available":42,"total_granted":50,"total_used":8}}`)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer up.Close()
			a := Adapter{HTTP: up.Client(), Config: Config{Platform: "newapi"}, BaseURL: up.URL, Key: "test-key"}
			v := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: "balance-summary"})
			if v.Failure != tc.fail {
				t.Fatal(v)
			}
			if tc.fallback {
				if calls != 2 || v.Balance.Scope != "key" || v.Balance.Remaining.Value.String() != "42" {
					t.Fatal(v, calls)
				}
			} else if calls != 1 || v.Balance != nil {
				t.Fatal(v, calls)
			}
		})
	}
}

func TestNewAPIBillingUnlimitedPlaceholder(t *testing.T) {
	for _, op := range []string{"balance", "balance-summary"} {
		up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/dashboard/billing/subscription" {
				fmt.Fprint(w, `{"hard_limit_usd":100000000}`)
			} else if r.URL.Path == "/api/usage/token/" {
				fmt.Fprint(w, `{"code":true,"data":{"total_available":0,"total_granted":0,"total_used":0,"unlimited_quota":true}}`)
			} else {
				t.Error("unexpected path", r.URL.Path)
			}
		}))
		a := Adapter{HTTP: up.Client(), Config: Config{Platform: "newapi"}, BaseURL: up.URL, Key: "test-key"}
		v := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: op})
		up.Close()
		if v.Failure != "" || v.Balance.Scope != "key" || v.Balance.Unlimited == nil || !*v.Balance.Unlimited {
			t.Fatal(v)
		}
		if v.Balance.Limit != nil && v.Balance.Limit.Value.String() == "100000000" {
			t.Fatal("placeholder exposed as funds")
		}
	}
}

func TestNewAPIBillingMalformedUsageAndOptionalMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, total, usage, fail, unit string
		status                         int
	}{
		{"missing usage", "12", `{}`, "invalid_response", "", 200},
		{"null usage", "12", `{"total_usage":null}`, "invalid_response", "", 200},
		{"negative usage", "12", `{"total_usage":-1}`, "invalid_response", "", 200},
		{"invalid total", "null", `{"total_usage":0}`, "invalid_response", "", 200},
		{"negative total", "-1", `{"total_usage":0}`, "invalid_response", "", 200},
		{"bounded exponent", "1e-1000000000", `{"total_usage":0}`, "invalid_response", "", 200},
		{"usage rejected", "12", `{"error":{"message":"rejected"}}`, "upstream_rejected", "", 200},
		{"missing metadata", "12", `{"total_usage":0}`, "", "site_unit", 404},
		{"metadata denied", "12", `{"total_usage":0}`, "", "site_unit", 403},
		{"metadata limited", "12", `{"total_usage":0}`, "rate_limited", "", 429},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/dashboard/billing/subscription":
					fmt.Fprintf(w, `{"hard_limit_usd":%s}`, tc.total)
				case "/v1/dashboard/billing/usage":
					fmt.Fprint(w, tc.usage)
				case "/api/status":
					w.WriteHeader(tc.status)
					fmt.Fprint(w, `{"success":true,"data":{}}`)
				default:
					t.Error("unexpected path", r.URL.Path)
				}
			}))
			defer up.Close()
			a := Adapter{HTTP: up.Client(), Config: Config{Platform: "newapi"}, BaseURL: up.URL, Key: "test-key"}
			v := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: "balance"})
			if v.Failure != tc.fail {
				t.Fatal(v)
			}
			if tc.fail != "" && v.Balance != nil {
				t.Fatal("fabricated allowance", v)
			}
			if tc.fail == "" && (v.Balance.Remaining.Value.String() != "12" || v.Balance.Remaining.Unit != tc.unit || v.HTTPStatus != 200) {
				t.Fatal(v)
			}
		})
	}
}
