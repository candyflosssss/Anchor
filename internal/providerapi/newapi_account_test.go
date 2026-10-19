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

func TestNewAPIAccountSummaryUsesSavedSystemToken(t *testing.T) {
	for _, platform := range []string{"newapi", "auto"} {
		for _, userID := range []int64{0, 12} {
			for _, key := range []string{"", "inference-fixture"} {
				t.Run(fmt.Sprintf("%s/id-%d/key-%t", platform, userID, key != ""), func(t *testing.T) {
					var paths []string
					up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						paths = append(paths, r.URL.Path)
						if r.Method != http.MethodGet {
							t.Error("unexpected method", r.Method)
						}
						switch r.URL.Path {
						case "/prefix/api/status":
							if r.Header.Get("Authorization") != "" || r.Header.Get("New-Api-User") != "" {
								t.Error("credential in public probe")
							}
							fmt.Fprint(w, `{"success":true,"data":{"version":"1","quota_per_unit":500000,"default_use_auto_group":true,"enable_batch_update":false}}`)
						case "/prefix/api/user/self":
							if r.Header.Get("Authorization") != "Bearer system-token-fixture" {
								t.Error("account queried with inference credential")
							}
							wantID := ""
							if userID > 0 {
								wantID = remoteID(userID)
							}
							if r.Header.Get("New-Api-User") != wantID {
								t.Error("incorrect user header")
							}
							fmt.Fprint(w, `{"success":true,"data":{"quota":9007199254740993123,"used_quota":12}}`)
						default:
							t.Error("account summary queried a key endpoint", r.URL.Path)
							w.WriteHeader(404)
						}
					}))
					defer up.Close()
					a := Adapter{HTTP: up.Client(), BaseURL: up.URL + "/prefix/v1", Key: key, Config: Config{Platform: platform, UserToken: "system-token-fixture", UserID: userID}}
					v := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: "balance-summary"})
					if v.Platform != "newapi" || v.Failure != "" || v.Balance == nil || v.Balance.Scope != "account" || v.Balance.Remaining == nil || v.Balance.Remaining.Value.String() != "9007199254740993123" || v.Balance.Remaining.Unit != "quota" {
						t.Fatalf("result=%+v balance=%+v", v, v.Balance)
					}
					wantPaths := []string{"/prefix/api/user/self", "/prefix/api/status"}
					if platform == "auto" {
						wantPaths = []string{"/prefix/api/status", "/prefix/api/user/self"}
					}
					if !reflect.DeepEqual(paths, wantPaths) {
						t.Fatal(paths)
					}
				})
			}
		}
	}
}

func TestNewAPIAccountSummaryPreservesFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body, fail string
		status           int
	}{
		{"zero wallet", `{"success":true,"data":{"quota":0}}`, "", 200},
		{"expired token", `{"success":false}`, "authentication", 401},
		{"missing profile permission", `{"success":false}`, "permission_denied", 403},
		{"missing account endpoint", "", "unsupported", 404},
		{"rate limit", "", "rate_limited", 429},
		{"server error", "", "http_error", 500},
		{"login page", "<html>login</html>", "invalid_response", 200},
		{"missing quota", `{"success":true,"data":{}}`, "invalid_response", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path == "/api/status" {
					w.WriteHeader(404)
					return
				}
				if r.URL.Path != "/api/user/self" || r.Header.Get("Authorization") != "Bearer system-token-fixture" {
					t.Error("account failure fell back to a key query", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer up.Close()
			a := Adapter{HTTP: up.Client(), BaseURL: up.URL, Key: "inference-fixture", Config: Config{Platform: "newapi", UserToken: "system-token-fixture"}}
			v := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: "balance-summary"})
			wantCalls := 1
			if tc.fail == "" {
				wantCalls = 2
			}
			if v.Failure != tc.fail || calls != wantCalls {
				t.Fatalf("result=%+v calls=%d", v, calls)
			}
			if tc.fail != "" && v.Balance != nil {
				t.Fatal("failure exposed a fabricated allowance", v)
			}
			if tc.fail == "" && (v.Balance == nil || v.Balance.Remaining == nil || v.Balance.Remaining.Value.String() != "0" || v.Balance.Scope != "account") {
				t.Fatal("zero wallet not preserved", v)
			}
			if strings.Contains(fmt.Sprintf("%+v", v), "system-token-fixture") {
				t.Fatal("credential leaked")
			}
		})
	}
}
