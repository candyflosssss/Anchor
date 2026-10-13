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
		Settings   AccountSettings    `json:"settings"`
		Balance    AccountObservation `json:"balance"`
		Quota      AccountObservation `json:"quota"`
		Summary    AccountObservation `json:"summary"`
		KeySummary AccountObservation `json:"key_summary"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(v)
	var a, b any
	json.Unmarshal(raw, &a)
	json.Unmarshal(encoded, &b)
	if !reflect.DeepEqual(a, b) || v.Balance.Allowance.Remaining.Value != "12.34567890123456789" || v.Balance.Allowance.Used != nil || v.Quota.Allowance.Unlimited == nil || !*v.Quota.Allowance.Unlimited {
		t.Fatal("account fixture drift")
	}
}
func TestAccountBridgeCoreUpstream(t *testing.T) {
	for _, platform := range []string{"sub2api", "newapi", "oneapi"} {
		t.Run(platform, func(t *testing.T) {
			key, token := "synthetic-inference-secret", "synthetic-user-credential"
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
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
			saved := b.SaveProvider(SaveProvider{Provider: Provider{ID: "account", BaseURLs: []string{upstream.URL + "/proxy/v1"}, Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, Secret: &key})
			if saved.Problem != nil {
				t.Fatal(saved)
			}
			empty := b.ProviderAccountSettings(WorkspaceQuery{ProviderID: "account"})
			if empty.Problem != nil || empty.Data.Platform != "" || empty.Data.Revision != 1 {
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
	saved := b.SaveProvider(SaveProvider{Provider: Provider{ID: "summary", BaseURLs: []string{upstream.URL + "/prefix/v1"}, Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, Secret: &key})
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
