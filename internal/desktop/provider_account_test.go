package desktop

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSharedAccountFixture(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/provider-account.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		KeyBillingRate       AccountRateObservation `json:"key_billing_rate"`
		NewAPIAccountSummary AccountObservation     `json:"newapi_account_summary"`
		Billing              AccountObservation     `json:"billing"`
		BillingSummary       AccountObservation     `json:"billing_summary"`
		Rate                 AccountRateObservation `json:"rate"`
		Settings             AccountSettings        `json:"settings"`
		Balance              AccountObservation     `json:"balance"`
		Quota                AccountObservation     `json:"quota"`
		Summary              AccountObservation     `json:"summary"`
		KeySummary           AccountObservation     `json:"key_summary"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(v)
	var a, b any
	json.Unmarshal(raw, &a)
	json.Unmarshal(encoded, &b)
	if v.Billing.Allowance == nil || v.Billing.Allowance.Scope != "billing" || v.BillingSummary.Operation != "balance-summary" {
		t.Fatal("billing fixture drift")
	}
	if v.NewAPIAccountSummary.Platform != "newapi" || v.NewAPIAccountSummary.Operation != "balance-summary" || v.NewAPIAccountSummary.Allowance == nil || v.NewAPIAccountSummary.Allowance.Scope != "account" {
		t.Fatal("NewAPI account summary fixture drift")
	}
	if v.KeyBillingRate.Platform != "sub2api" || v.KeyBillingRate.Rate == nil || v.KeyBillingRate.Rate.Basis != "key_billing" || v.KeyBillingRate.Rate.Group != "" {
		t.Fatal("key billing fixture drift")
	}
	if !reflect.DeepEqual(a, b) || v.Balance.Allowance.Remaining.Value != "12.34567890123456789" || v.Balance.Allowance.Used != nil || v.Quota.Allowance.Unlimited == nil || !*v.Quota.Allowance.Unlimited {
		t.Fatal("account fixture drift")
	}
}

func TestDetectPlatformBridgeSavesAndReadsCachedIdentity(t *testing.T) {
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/status" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected detector request", r.URL.Path)
		}
		fmt.Fprint(w, `{"data":{"version":"1","quota_per_unit":500000,"default_use_auto_group":true,"enable_batch_update":false}}`)
	}))
	defer up.Close()
	b := NewBridge(testSession(t))
	requireReady(t, b)
	key := "synthetic-bridge-detect"
	saved := b.SaveProvider(SaveProvider{Provider: Provider{ID: "detected", BaseURLs: []string{up.URL + "/v1"}, Enabled: false, TimeoutSeconds: 5, MaxConcurrent: 1}, Secret: &key})
	if saved.Problem != nil || calls != 1 {
		t.Fatal(saved, calls)
	}
	settings := b.ProviderAccountSettings(WorkspaceQuery{ProviderID: "detected"})
	if settings.Problem != nil || settings.Data.DetectedPlatform != "newapi" || calls != 1 {
		t.Fatal(settings, calls)
	}
	q := AccountQuery{ProviderID: "detected", ExpectedRevision: 1, Operation: "detect-platform"}
	detected := b.DetectProviderPlatform(q)
	if detected.Problem != nil || detected.Data.DetectedPlatform != "newapi" || detected.Data.Revision != 1 || calls != 2 {
		t.Fatal(detected, calls)
	}
	q.ExpectedRevision = 2
	stale := b.DetectProviderPlatform(q)
	if stale.Problem == nil || stale.Problem.Code != "conflict" || calls != 2 {
		t.Fatal(stale, calls)
	}
	q.ExpectedRevision = 1
	q.Operation = "balance"
	if invalid := b.DetectProviderPlatform(q); invalid.Problem == nil || invalid.Problem.Code != "invalid" {
		t.Fatal(invalid)
	}
}
func TestAccountBridgeCoreUpstream(t *testing.T) {
	for _, platform := range []string{"sub2api", "newapi", "oneapi"} {
		t.Run(platform, func(t *testing.T) {
			key, token := "synthetic-inference-secret", "synthetic-user-credential"
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/proxy/v1/sub2api/billing", "/proxy/api/v1/keys", "/proxy/api/token/":
					w.WriteHeader(404)
				case "/proxy/v1/usage":
					if r.Header.Get("Authorization") != "Bearer "+key {
						t.Error("wrong inference credential")
					}
					fmt.Fprint(w, `{"mode":"unrestricted","balance":12.34567890123456789}`)
				case "/proxy/api/v1/user/profile", "/proxy/api/user/self", "/proxy/api/token/42":
					if r.Header.Get("Authorization") != "Bearer "+token {
						t.Error("wrong user credential")
					}
					if platform == "newapi" && r.Header.Get("New-Api-User") != "7" {
						t.Error("missing user ID")
					}
					fmt.Fprint(w, `{"success":true,"data":{"balance":8.12,"quota":9007199254740993123,"used_quota":17,"remain_quota":1500,"unlimited_quota":false}}`)
				case "/proxy/v1/dashboard/billing/subscription", "/proxy/v1/dashboard/billing/usage":
					if r.Header.Get("Authorization") != "Bearer "+key || r.Header.Get("New-Api-User") != "" {
						t.Error("wrong billing credential")
					}
					if strings.HasSuffix(r.URL.Path, "subscription") {
						fmt.Fprint(w, `{"hard_limit_usd":20.12}`)
					} else {
						fmt.Fprint(w, `{"total_usage":100}`)
					}
				case "/proxy/api/v1/settings/public":
					fmt.Fprint(w, `{"data":{"registration_enabled":true,"site_name":"fixture","version":"1","api_base_url":""}}`)
				case "/proxy/api/status":
					if r.Header.Get("Authorization") != "" {
						t.Error("public credential")
					}
					fmt.Fprint(w, `{"success":true,"data":{"quota_display_type":"USD"}}`)
				case "/proxy/api/usage/token/":
					if r.Header.Get("Authorization") != "Bearer "+key {
						t.Error("wrong quota credential")
					}
					fmt.Fprint(w, `{"success":true,"data":{"total_available":1234,"total_used":3,"total_granted":1237,"unlimited_quota":false}}`)
				default:
					t.Errorf("unexpected upstream path %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			b := NewBridge(testSession(t))
			requireReady(t, b)
			saved := b.SaveProvider(SaveProvider{Provider: Provider{ID: "account", BaseURLs: []string{upstream.URL + "/proxy/v1"}, Enabled: false, TimeoutSeconds: 5, MaxConcurrent: 1}, Secret: &key})
			if saved.Problem != nil {
				t.Fatal(saved)
			}
			empty := b.ProviderAccountSettings(WorkspaceQuery{ProviderID: "account"})
			if empty.Problem != nil || empty.Data.Platform != "auto" || empty.Data.Revision != 1 {
				t.Fatal(empty)
			}
			cfg := SaveAccountSettings{ProviderID: "account", ExpectedRevision: 1, Platform: platform, UserID: 7}
			v := b.SaveProviderAccount(cfg)
			if v.Problem != nil || v.Data.Revision != 2 || v.Data.HasUserToken {
				t.Fatal(v)
			}
			q := AccountQuery{ProviderID: "account", ExpectedRevision: 2, Operation: "balance"}
			wallet := b.ProviderAllowance(q)
			if wallet.Problem != nil {
				t.Fatal(wallet)
			}
			if platform == "sub2api" {
				if wallet.Data.Allowance == nil || wallet.Data.Allowance.Remaining.Value != "12.34567890123456789" || wallet.Data.Allowance.Scope != "account" {
					t.Fatal(wallet)
				}
			} else if platform == "newapi" {
				if wallet.Data.Failure != "" || wallet.Data.Allowance == nil || wallet.Data.Allowance.Scope != "billing" || wallet.Data.Allowance.Remaining.Value != "19.12" || wallet.Data.Allowance.Remaining.Unit != "USD" {
					t.Fatal(wallet)
				}
			} else if wallet.Data.Failure != "auth_required" || wallet.Data.Allowance != nil {
				t.Fatal(wallet)
			}
			cfg.ExpectedRevision = 2
			cfg.UserToken = &token
			v = b.SaveProviderAccount(cfg)
			if v.Problem != nil || v.Data.Revision != 3 || !v.Data.HasUserToken {
				t.Fatal(v)
			}
			q.ExpectedRevision = 3
			wallet = b.ProviderAllowance(q)
			if wallet.Problem != nil || wallet.Data.Failure != "" || wallet.Data.Allowance == nil {
				t.Fatal(wallet)
			}
			if platform != "sub2api" && (wallet.Data.Allowance.Remaining.Value != "9007199254740993123" || wallet.Data.Allowance.Remaining.Unit != "quota") {
				t.Fatal(wallet)
			}
			q.Operation = "key-quota"
			quota := b.ProviderAllowance(q)
			if platform == "oneapi" {
				if quota.Problem != nil || quota.Data.Failure != "remote_key_required" {
					t.Fatal(quota)
				}
				q.RemoteKeyID = 42
				quota = b.ProviderAllowance(q)
			}
			if quota.Problem != nil || quota.Data.Failure != "" || quota.Data.Allowance == nil || quota.Data.Allowance.Scope != "key" {
				t.Fatal(quota)
			}
			if platform == "sub2api" && (quota.Data.Allowance.Unlimited == nil || !*quota.Data.Allowance.Unlimited || quota.Data.Allowance.Remaining != nil) {
				t.Fatal(quota)
			}
			conflict := b.SaveProviderAccount(cfg)
			if conflict.Problem == nil || conflict.Problem.Code != "conflict" {
				t.Fatal(conflict)
			}
			q.ExpectedRevision = 2
			stale := b.ProviderAllowance(q)
			if stale.Problem == nil || stale.Problem.Code != "conflict" {
				t.Fatal(stale)
			}
			raw, _ := json.Marshal(v)
			if strings.Contains(string(raw), token) || strings.Contains(string(raw), key) {
				t.Fatal("credential echoed")
			}
			cfg.ExpectedRevision = 3
			cfg.UserToken = nil
			cfg.SiteURL = upstream.URL + "/other"
			moved := b.SaveProviderAccount(cfg)
			if moved.Problem != nil || moved.Data.HasUserToken {
				t.Fatal("old auth retained on changed target", moved)
			}
		})
	}
}
func TestAccountBridgeRejectsUnsafeInputs(t *testing.T) {
	b := NewBridge(testSession(t))
	requireReady(t, b)
	for _, q := range []AccountQuery{
		{ProviderID: "p", ExpectedRevision: 1, Operation: "usage-records"},
		{ProviderID: "p", ExpectedRevision: 1, Operation: "balance", RemoteKeyID: MaxSafeRevision + 1},
		{ProviderID: "p", ExpectedRevision: MaxSafeRevision + 1, Operation: "balance"},
	} {
		out := b.ProviderAllowance(q)
		if out.Problem == nil || out.Problem.Code != "invalid" {
			t.Fatal(out)
		}
	}
	out := b.SaveProviderAccount(SaveAccountSettings{ProviderID: "p", ExpectedRevision: 1, Platform: "newapi", UserID: MaxSafeRevision + 1})
	if out.Problem == nil || out.Problem.Code != "invalid" {
		t.Fatal(out)
	}
}

func TestAccountBridgeAutomaticallyDetectsSummary(t *testing.T) {
	key := "default-summary-fixture"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prefix/api/status":
			if r.Header.Get("Authorization") != "" {
				t.Error("public request credential")
			}
			fmt.Fprint(w, `{"success":true,"data":{"version":"1","quota_per_unit":500000,"default_use_auto_group":true,"enable_batch_update":false}}`)
		case "/prefix/v1/dashboard/billing/subscription":
			w.WriteHeader(404)
		case "/prefix/api/usage/token/":
			if r.Header.Get("Authorization") != "Bearer "+key {
				t.Error("wrong key")
			}
			fmt.Fprint(w, `{"code":true,"data":{"total_available":9007199254740993123,"total_used":2,"total_granted":9007199254740993125}}`)
		default:
			t.Error("unexpected path")
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	b := NewBridge(testSession(t))
	requireReady(t, b)
	saved := b.SaveProvider(SaveProvider{Provider: Provider{ID: "summary", BaseURLs: []string{upstream.URL + "/prefix/v1"}, Enabled: false, TimeoutSeconds: 5, MaxConcurrent: 1}, Secret: &key})
	if saved.Problem != nil {
		t.Fatal(saved)
	}
	q := AccountQuery{ProviderID: "summary", ExpectedRevision: 1, Operation: "balance-summary"}
	v := b.ProviderAllowance(q)
	if v.Problem != nil || v.Data.Failure != "" || v.Data.Platform != "newapi" || v.Data.KeyID != "default" || v.Data.Allowance.Scope != "key" || v.Data.Allowance.Remaining.Value != "9007199254740993123" {
		t.Fatalf("%+v", v)
	}
	settings := b.SaveProviderAccount(SaveAccountSettings{ProviderID: "summary", ExpectedRevision: 1, Platform: "auto"})
	if settings.Problem != nil || settings.Data.Platform != "auto" {
		t.Fatal(settings)
	}
	if stale := b.ProviderAllowance(q); stale.Problem == nil || stale.Problem.Code != "conflict" {
		t.Fatal(stale)
	}
}

func TestRateBridgeCoreUpstream(t *testing.T) {
	for _, tc := range []struct {
		name, platform, group, failure string
		authorized, personal           bool
	}{
		{"personal", "sub2api", "7", "", true, true},
		{"default", "sub2api", "7", "", true, false},
		{"newapi", "newapi", "vip", "", true, false},
		{"auto", "newapi", "auto", "unknown", true, false},
		{"missing-auth", "sub2api", "7", "auth_required", false, false},
		{"oneapi", "oneapi", "", "unsupported", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, token := "synthetic-inference-123", "synthetic-user-jwt"
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/proxy/api/status" {
					w.WriteHeader(404)
					return
				}
				if r.URL.Path == "/proxy/api/v1/settings/public" {
					fmt.Fprint(w, `{"data":{"registration_enabled":true,"site_name":"fixture","version":"1","api_base_url":""}}`)
					return
				}
				calls++
				if r.URL.Path == "/proxy/v1/sub2api/billing" {
					if r.Header.Get("Authorization") != "Bearer "+key {
						t.Error("incorrect inference authorization")
					}
					w.WriteHeader(404)
					return
				}
				if r.Header.Get("Authorization") != "Bearer "+token {
					t.Error("incorrect user authorization")
				}
				if tc.platform == "newapi" && r.Header.Get("New-Api-User") != "9" {
					t.Error("missing user ID")
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/proxy/api/v1/keys":
					fmt.Fprintf(w, `{"data":{"items":[{"id":42,"key":%q}],"total":1}}`, key)
				case "/proxy/api/token/":
					fmt.Fprint(w, `{"success":true,"data":{"items":[{"id":42,"key":"synthetic*****-123"}],"total":1}}`)
				case "/proxy/api/token/42/key":
					if r.Method != "POST" {
						t.Error("key read must be POST")
					}
					fmt.Fprintf(w, `{"success":true,"data":{"key":%q}}`, key)
				case "/proxy/api/v1/keys/42":
					fmt.Fprint(w, `{"data":{"group_id":7,"group":{"name":"标准组"}}}`)
				case "/proxy/api/token/42":
					fmt.Fprintf(w, `{"success":true,"data":{"group":%q}}`, tc.group)
				case "/proxy/api/v1/groups/rates":
					if tc.personal {
						fmt.Fprint(w, `{"data":{"7":0.80000000000000001}}`)
					} else {
						fmt.Fprint(w, `{"data":null}`)
					}
				case "/proxy/api/v1/groups/available":
					fmt.Fprint(w, `{"data":[{"id":7,"name":"标准组","rate_multiplier":0.80000000000000001}]}`)
				case "/proxy/api/pricing":
					fmt.Fprint(w, `{"success":true,"group_ratio":{"vip":0.80000000000000001}}`)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			b := NewBridge(testSession(t))
			requireReady(t, b)
			saved := b.SaveProvider(SaveProvider{Provider: Provider{ID: "rates", BaseURLs: []string{upstream.URL + "/proxy/v1"}, Enabled: false, TimeoutSeconds: 5, MaxConcurrent: 1}, Secret: &key})
			if saved.Problem != nil {
				t.Fatal(saved)
			}
			calls = 0
			cfg := SaveAccountSettings{ProviderID: "rates", ExpectedRevision: 1, Platform: tc.platform, UserID: 9}
			if tc.authorized {
				cfg.UserToken = &token
			}
			configured := b.SaveProviderAccount(cfg)
			if configured.Problem != nil {
				t.Fatal(configured)
			}
			q := AccountQuery{ProviderID: "rates", ExpectedRevision: 2, Operation: "effective-rate"}
			result := b.ProviderRate(q)
			if result.Problem != nil || result.Data == nil {
				t.Fatal(result)
			}
			v := result.Data
			if v.Failure != tc.failure || v.Revision != 2 || v.KeyID != "default" || v.RemoteKeyID != 0 {
				t.Fatalf("unexpected rate %+v", v)
			}
			if tc.failure == "" {
				if v.Rate == nil || v.Rate.Multiplier == nil || *v.Rate.Multiplier != "0.80000000000000001" || v.Rate.Group != tc.group {
					t.Fatalf("rate %+v", v.Rate)
				}
				basis := "authenticated_group"
				if tc.personal {
					basis = "personal_override"
				}
				if v.Rate.Basis != basis {
					t.Fatal(v.Rate)
				}
			} else if v.Rate != nil && v.Rate.Multiplier != nil {
				t.Fatal("failed query returned a rate")
			}
			expectedUnauthorizedCalls := 0
			if tc.platform == "sub2api" {
				expectedUnauthorizedCalls = 1
			}
			if !tc.authorized && calls != expectedUnauthorizedCalls {
				t.Fatal("unexpected key-only probe count")
			}
			if tc.name == "auto" && (v.Rate == nil || v.Rate.Group != "auto") {
				t.Fatal("lost unknown group")
			}
			raw, _ := json.Marshal(result)
			if strings.Contains(string(raw), key) || strings.Contains(string(raw), token) {
				t.Fatal("secret echoed")
			}
			q.ExpectedRevision = 1
			stale := b.ProviderRate(q)
			if stale.Problem == nil || stale.Problem.Code != "conflict" {
				t.Fatal(stale)
			}
			for _, bad := range []AccountQuery{{ProviderID: "rates", ExpectedRevision: 2, Operation: "balance"}, {ProviderID: "rates", ExpectedRevision: 2, Operation: "effective-rate", RemoteKeyID: MaxSafeRevision + 1}} {
				invalid := b.ProviderRate(bad)
				if invalid.Problem == nil || invalid.Problem.Code != "invalid" {
					t.Fatal(invalid)
				}
			}
		})
	}
}

func TestSub2KeyBillingThroughDesktopCatalogAndAccount(t *testing.T) {
	first, second := "synthetic-billing-first", "synthetic-billing-second"
	billingCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxy/api/status" {
			w.WriteHeader(404)
			return
		}
		if r.URL.Path == "/proxy/api/v1/settings/public" {
			fmt.Fprint(w, `{"data":{"registration_enabled":true,"site_name":"fixture","version":"1","api_base_url":""}}`)
			return
		}
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if r.Method != "GET" || (key != first && key != second) || r.URL.RawQuery != "" {
			t.Error("invalid key-only request")
		}
		switch r.URL.Path {
		case "/proxy/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"shared-model"}]}`)
		case "/proxy/v1/sub2api/billing":
			billingCalls++
			rate := "1.2"
			if key == second {
				rate = "0"
			}
			fmt.Fprintf(w, `{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":2,"resolved_rate_multiplier":0.8,"peak_rate_enabled":true,"applied_peak_multiplier":1.5,"effective_rate_multiplier":%s}`, rate)
		default:
			t.Error("unexpected endpoint", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	b := NewBridge(testSession(t))
	requireReady(t, b)
	saved := b.SaveProvider(SaveProvider{Provider: Provider{ID: "key-billing", BaseURLs: []string{upstream.URL + "/proxy/v1"}, Enabled: false, TimeoutSeconds: 5, MaxConcurrent: 1}, Secret: &first})
	if saved.Problem != nil {
		t.Fatal(saved)
	}
	added := b.ProviderAction(Action{ProviderID: "key-billing", ExpectedRevision: 1, Action: "key-put", Target: "second", Secret: second, Protocol: "responses", Enabled: true})
	if added.Problem != nil {
		t.Fatal(added)
	}
	catalog := b.DiscoverProviderModels(DiscoverProviderModels{ProviderID: "key-billing", ExpectedRevision: 2})
	if catalog.Problem != nil || catalog.Data.Failure != "" || len(catalog.Data.Models) != 1 || len(catalog.Data.Keys) != 2 || len(catalog.Data.Entries[0].KeyIDs) != 2 || billingCalls != 2 {
		t.Fatal(catalog, billingCalls)
	}
	for i, k := range catalog.Data.Keys {
		want := "1.2"
		if i == 1 {
			want = "0"
		}
		if k.Multiplier == nil || *k.Multiplier != want || k.RateBasis != "key_billing" || k.Group != "" || k.GroupName != "" || k.RateFailure != "" {
			t.Fatalf("invalid catalog rate %+v", k)
		}
	}
	for _, id := range []string{"default", "second"} {
		result := b.ProviderRate(AccountQuery{ProviderID: "key-billing", ExpectedRevision: 2, Operation: "effective-rate", KeyID: id})
		want := "1.2"
		if id == "second" {
			want = "0"
		}
		if result.Problem != nil || result.Data == nil || result.Data.Failure != "" || result.Data.Platform != "sub2api" || result.Data.Rate == nil || result.Data.Rate.Multiplier == nil || *result.Data.Rate.Multiplier != want || result.Data.Rate.Basis != "key_billing" || result.Data.Rate.Group != "" {
			t.Fatal(result)
		}
		raw, _ := json.Marshal(result)
		if strings.Contains(string(raw), first) || strings.Contains(string(raw), second) {
			t.Fatal("secret leaked")
		}
	}
	settings := b.ProviderAccountSettings(WorkspaceQuery{ProviderID: "key-billing"})
	if settings.Problem != nil || settings.Data.HasUserToken || billingCalls != 4 {
		t.Fatal(settings, billingCalls)
	}
}
