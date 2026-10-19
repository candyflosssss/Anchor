package providerapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestNewAPIWalletQuotaConversion(t *testing.T) {
	for _, tc := range []struct{ name, metadata, value, unit string }{
		{"USD", `"quota_display_type":"USD","quota_per_unit":500000`, "12.345678", "USD"},
		{"CNY", `"quota_display_type":"CNY","quota_per_unit":500000,"usd_exchange_rate":7.2`, "88.8888816", "CNY"},
		{"custom divisor", `"quota_display_type":"USD","quota_per_unit":1000`, "6172.839", "USD"},
		{"tokens", `"quota_display_type":"TOKENS","quota_per_unit":500000`, "6172839", "quota"},
		{"missing divisor", `"quota_display_type":"USD"`, "6172839", "quota"},
		{"zero divisor", `"quota_display_type":"USD","quota_per_unit":0`, "6172839", "quota"},
		{"missing exchange", `"quota_display_type":"CNY","quota_per_unit":500000`, "6172839", "quota"},
		{"unknown currency", `"quota_display_type":"CUSTOM","quota_per_unit":500000`, "6172839", "quota"},
	} {
		for _, credential := range []string{"account", "key"} {
			t.Run(tc.name+"/"+credential, func(t *testing.T) {
				var paths []string
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					paths = append(paths, r.URL.Path)
					switch r.URL.Path {
					case "/prefix/api/status":
						if r.Header.Get("Authorization") != "" || r.Header.Get("New-Api-User") != "" {
							t.Error("credential sent to public endpoint")
						}
						fmt.Fprintf(w, `{"success":true,"data":{%s}}`, tc.metadata)
					case "/prefix/api/user/self":
						if credential != "account" || r.Header.Get("Authorization") != "Bearer account-fixture" || r.Header.Get("New-Api-User") != "7" {
							t.Error("wrong account authorization")
						}
						fmt.Fprint(w, `{"success":true,"data":{"quota":6172839,"used_quota":500000}}`)
					case "/prefix/api/usage/token/":
						if credential != "key" || r.Header.Get("Authorization") != "Bearer key-fixture" || r.Header.Get("New-Api-User") != "" {
							t.Error("wrong key authorization")
						}
						// A key can have no cap while the wallet is finite. The wallet
						// must take priority over unlimited_quota and the key figures.
						fmt.Fprint(w, `{"code":true,"data":{"total_available":1,"total_used":2,"total_granted":3,"unlimited_quota":true,"user_quota":{"total_available":6172839,"total_used":500000}}}`)
					default:
						t.Error("unexpected fallback", r.URL.Path)
						w.WriteHeader(404)
					}
				}))
				defer up.Close()
				a := Adapter{HTTP: up.Client(), BaseURL: up.URL + "/prefix/v1", Key: "key-fixture", Config: Config{Platform: "newapi", UserID: 7}}
				if credential == "account" {
					a.Config.UserToken = "account-fixture"
				}
				v := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: "balance-summary"})
				if v.Failure != "" || v.HTTPStatus != 200 || v.Balance == nil || v.Balance.Scope != "account" || v.Balance.Unlimited != nil || v.Balance.Remaining.Value.String() != tc.value || v.Balance.Remaining.Unit != tc.unit || v.Balance.Used == nil || v.Balance.Used.Unit != tc.unit {
					t.Fatalf("result=%+v balance=%+v", v, v.Balance)
				}
				first := "/prefix/api/usage/token/"
				if credential == "account" {
					first = "/prefix/api/user/self"
				}
				if !reflect.DeepEqual(paths, []string{first, "/prefix/api/status"}) {
					t.Fatal(paths)
				}
			})
		}
	}
}

func TestNewAPIReadOnlyKeySurvivesBillingRejection(t *testing.T) {
	for _, status := range []int{401, 403, 404} {
		for _, op := range []string{"balance", "balance-summary", "key-quota"} {
			t.Run(fmt.Sprintf("%d/%s", status, op), func(t *testing.T) {
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/usage/token/":
						fmt.Fprint(w, `{"code":true,"data":{"total_available":0,"total_used":500000,"total_granted":500000,"unlimited_quota":false}}`)
					case "/api/status":
						fmt.Fprint(w, `{"success":true,"data":{"quota_display_type":"USD","quota_per_unit":500000}}`)
					case "/v1/dashboard/billing/subscription":
						w.WriteHeader(status)
					default:
						t.Error("unexpected path", r.URL.Path)
					}
				}))
				defer up.Close()
				a := Adapter{HTTP: up.Client(), BaseURL: up.URL, Key: "key-fixture", Config: Config{Platform: "newapi"}}
				v := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: op})
				out := v.Balance
				if op == "key-quota" {
					out = v.KeyQuota
				}
				if v.Failure != "" || v.HTTPStatus != 200 || out == nil || out.Scope != "key" || out.Remaining.Value.String() != "0" || out.Remaining.Unit != "USD" || out.Used.Value.String() != "1" || out.Limit.Value.String() != "1" {
					t.Fatalf("result=%+v allowance=%+v", v, out)
				}
			})
		}
	}
}

func TestNewAPIKeyQuotaDoesNotUseWallet(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/status" {
			fmt.Fprint(w, `{"success":true,"data":{"quota_display_type":"USD","quota_per_unit":500000}}`)
			return
		}
		fmt.Fprint(w, `{"code":true,"data":{"total_available":500000,"total_used":0,"total_granted":500000,"unlimited_quota":true,"user_quota":{"total_available":9000000,"total_used":0}}}`)
	}))
	defer up.Close()
	a := Adapter{HTTP: up.Client(), BaseURL: up.URL, Key: "key-fixture", Config: Config{Platform: "newapi"}}
	v := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: "key-quota"})
	if v.Failure != "" || v.KeyQuota == nil || v.KeyQuota.Scope != "key" || v.KeyQuota.Remaining.Value.String() != "1" || v.KeyQuota.Unlimited == nil || !*v.KeyQuota.Unlimited {
		t.Fatal(v)
	}
}

func TestNewAPIMalformedWalletCannotBecomeKeyBalance(t *testing.T) {
	for _, wallet := range []string{`{}`, `[]`, `{"total_available":"12"}`} {
		t.Run(wallet, func(t *testing.T) {
			calls := 0
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/api/usage/token/" {
					t.Error("malformed wallet fell back to another scope", r.URL.Path)
				}
				fmt.Fprintf(w, `{"code":true,"data":{"total_available":500000,"total_used":0,"total_granted":500000,"user_quota":%s}}`, wallet)
			}))
			defer up.Close()
			a := Adapter{HTTP: up.Client(), BaseURL: up.URL, Key: "key-fixture", Config: Config{Platform: "newapi"}}
			v := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: "balance-summary"})
			if v.Failure != "invalid_response" || v.Balance != nil || calls != 1 {
				t.Fatal(v, calls)
			}
		})
	}
}

func TestNewAPIAutomaticWalletReusesTokenProbe(t *testing.T) {
	var paths []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/usage/token/":
			fmt.Fprint(w, `{"code":true,"data":{"total_available":0,"total_used":0,"total_granted":0,"unlimited_quota":true,"user_quota":{"total_available":-1234,"total_used":0}}}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer up.Close()
	a := Adapter{HTTP: up.Client(), BaseURL: up.URL, Key: "key-fixture", Config: Config{Platform: "auto"}}
	v := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: "balance-summary"})
	if v.Failure != "" || v.Platform != "newapi" || v.Balance == nil || v.Balance.Scope != "account" || v.Balance.Remaining.Value.String() != "-1234" || v.Balance.Remaining.Unit != "quota" {
		t.Fatal(v)
	}
	tokens := 0
	for _, path := range paths {
		if path == "/api/usage/token/" {
			tokens++
		}
	}
	if tokens != 1 || len(paths) > 8 {
		t.Fatal("duplicated token probe or exceeded budget", paths)
	}
}
