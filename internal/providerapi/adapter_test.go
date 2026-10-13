package providerapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func runFixture(t *testing.T, platform string, q Query, auth bool, responses map[string]string, check func(*http.Request)) Result {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/proxy/") || r.Method != "GET" {
			t.Error("lost prefix or wrong verb", r.URL.Path)
		}
		path := strings.TrimPrefix(r.URL.Path, "/proxy")
		token := "model-key-fixture"
		if path != "/v1/usage" && path != "/api/usage/token/" && path != "/api/log/token" {
			token = "user-token-fixture"
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("wrong authorization on %s", path)
		}
		if check != nil {
			check(r)
		}
		body, ok := responses[path]
		if !ok {
			t.Errorf("unexpected path %s", path)
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	cfg := Config{Platform: platform, UserID: 12}
	if auth {
		cfg.UserToken = "user-token-fixture"
	}
	a := Adapter{HTTP: server.Client(), Config: cfg, BaseURL: server.URL + "/proxy/v1", Key: "model-key-fixture"}
	q.ProviderID = "fixture"
	q.ExpectedRevision = 3
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return a.Query(ctx, q)
}
func TestBalancesAreNotKeyOrSubscriptionFunds(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`{"mode":"unrestricted","balance":0,"remaining":0}`, ""},
		{`{"mode":"quota_limited","remaining":90,"quota":{"limit":100,"used":10,"remaining":90}}`, "auth_required"},
		{`{"mode":"unrestricted","remaining":30,"subscription":{"daily_limit_usd":50}}`, "auth_required"},
		{`{"mode":"unrestricted","balance":1.2345678901234567}`, ""},
	} {
		out := runFixture(t, "sub2api", Query{Operation: "balance"}, false, map[string]string{"/v1/usage": tc.body}, nil)
		if out.Failure != tc.want {
			t.Fatal(out)
		}
		if tc.want == "" && (out.Balance.Scope != "account" || out.Balance.Remaining == nil || out.Balance.Remaining.Unit != "USD") {
			t.Fatal(out)
		}
		if strings.Contains(tc.body, "1.234") && out.Balance.Remaining.Value.String() != "1.2345678901234567" {
			t.Fatal("amount precision lost")
		}
	}
	for _, platform := range []string{"sub2api", "newapi", "oneapi"} {
		path, body := "/api/user/self", `{"success":true,"data":{"quota":0,"used_quota":345}}`
		if platform == "sub2api" {
			path = "/api/v1/user/profile"
			body = `{"code":0,"data":{"balance":0}}`
		}
		out := runFixture(t, platform, Query{Operation: "balance"}, true, map[string]string{path: body}, nil)
		if out.Failure != "" || out.Balance.Remaining.Value.String() != "0" {
			t.Fatal(out)
		}
		if platform != "sub2api" && out.Balance.Remaining.Unit != "quota" {
			t.Fatal("guessed currency")
		}
	}
}
func TestKeyQuotaModesAndRawUnits(t *testing.T) {
	out := runFixture(t, "sub2api", Query{Operation: "key-quota"}, false, map[string]string{"/v1/usage": `{"mode":"quota_limited","quota":{"limit":20,"used":3,"remaining":17},"rate_limits":[{"window":"5h","limit":5,"used":1,"remaining":4,"reset_at":"2026-10-13T10:00:00Z"}]}`}, nil)
	if out.Failure != "" || out.KeyQuota.Remaining.Value.String() != "17" || len(out.KeyQuota.Windows) != 1 || out.KeyQuota.Windows[0].Period != "5h" {
		t.Fatal(out)
	}
	out = runFixture(t, "sub2api", Query{Operation: "key-quota"}, false, map[string]string{"/v1/usage": `{"mode":"unrestricted","balance":50}`}, nil)
	if out.Failure != "" || out.KeyQuota.Remaining != nil || out.KeyQuota.Unlimited == nil || !*out.KeyQuota.Unlimited {
		t.Fatal("wallet presented as key quota", out)
	}
	out = runFixture(t, "newapi", Query{Operation: "key-quota"}, false, map[string]string{"/api/usage/token/": `{"code":true,"message":"ok","data":{"total_available":9000000000000001,"total_used":3,"total_granted":9000000000000004,"unlimited_quota":false}}`}, nil)
	// Match the upstream boolean code envelope, including large integer quotas.
	if out.Failure != "" || out.KeyQuota.Remaining.Value.String() != "9000000000000001" || *out.KeyQuota.Unlimited {
		t.Fatal(out)
	}
	out = runFixture(t, "newapi", Query{Operation: "key-quota"}, false, map[string]string{"/api/usage/token/": `{"success":true,"data":{"total_available":9000000000000001,"total_used":3,"total_granted":9000000000000004,"unlimited_quota":true}}`}, nil)
	if out.Failure != "" || out.KeyQuota.Remaining.Value.String() != "9000000000000001" || out.KeyQuota.Remaining.Unit != "quota" || !*out.KeyQuota.Unlimited {
		t.Fatal(out)
	}
	out = runFixture(t, "oneapi", Query{Operation: "key-quota", RemoteKeyID: 8}, true, map[string]string{"/api/token/8": `{"success":true,"data":{"remain_quota":20,"used_quota":5,"unlimited_quota":false}}`}, nil)
	if out.Failure != "" || out.KeyQuota.Remaining.Value.String() != "20" || out.KeyQuota.Scope != "key" {
		t.Fatal(out)
	}
}
func TestSub2APIPersonalRateOverrides(t *testing.T) {
	for _, override := range []bool{false, true} {
		fixtures := map[string]string{"/api/v1/keys/5": `{"code":0,"data":{"group_id":9,"key":"DO-NOT-EXPOSE"}}`, "/api/v1/groups/rates": `{"code":0,"data":null}`, "/api/v1/groups/available": `{"code":0,"data":[{"id":9,"rate_multiplier":2.5}]}`}
		want := "2.5"
		basis := "authenticated_group"
		if override {
			fixtures["/api/v1/groups/rates"] = `{"code":0,"data":{"9":0.75}}`
			delete(fixtures, "/api/v1/groups/available")
			want = "0.75"
			basis = "personal_override"
		}
		out := runFixture(t, "sub2api", Query{Operation: "effective-rate", RemoteKeyID: 5}, true, fixtures, nil)
		if out.Failure != "" || out.EffectiveRate.Multiplier.String() != want || out.EffectiveRate.Basis != basis {
			t.Fatal(out)
		}
		b, _ := json.Marshal(out)
		if strings.Contains(string(b), "DO-NOT-EXPOSE") {
			t.Fatal("credential leaked")
		}
	}
}
func TestNewAPIRateResolvesKeyGroupAndUserOverrides(t *testing.T) {
	out := runFixture(t, "newapi", Query{Operation: "effective-rate", RemoteKeyID: 7}, true, map[string]string{"/api/token/7": `{"success":true,"data":{"group":""}}`, "/api/user/self": `{"success":true,"data":{"group":"vip"}}`, "/api/pricing": `{"success":true,"data":[],"group_ratio":{"vip":0.6}}`}, func(r *http.Request) {
		if r.Header.Get("New-Api-User") != "12" {
			t.Error("missing configured user ID")
		}
	})
	if out.Failure != "" || out.EffectiveRate.Group != "vip" || out.EffectiveRate.Multiplier.String() != "0.6" {
		t.Fatal(out)
	}
	out = runFixture(t, "newapi", Query{Operation: "effective-rate", RemoteKeyID: 7}, true, map[string]string{"/api/token/7": `{"success":true,"data":{"group":"auto"}}`}, nil)
	if out.Failure != "unknown" {
		t.Fatal("auto-group presented as one rate", out)
	}
}
func TestRequestRecordsNormalizeBillingAndPagination(t *testing.T) {
	sub := `{"code":0,"data":{"total":3,"items":[{"id":12,"request_id":"req-sub","model":"m","created_at":"2026-10-13T10:00:00Z","api_key_id":5,"group_id":9,"input_tokens":100,"output_tokens":7,"cache_creation_tokens":10,"cache_read_tokens":20,"cache_creation_5m_tokens":8,"cache_creation_1h_tokens":2,"input_cost":0.4,"total_cost":1,"actual_cost":0.6,"rate_multiplier":0.6}]}}`
	out := runFixture(t, "sub2api", Query{Operation: "usage-records", Scope: "key", RemoteKeyID: 5, Page: 2, PageSize: 1}, true, map[string]string{"/api/v1/usage": sub}, func(r *http.Request) {
		if r.URL.Query().Get("api_key_id") != "5" || r.URL.Query().Get("page") != "2" {
			t.Error("wrong key/page filter")
		}
	})
	if out.Failure != "" || !out.Records.HasMore || out.Records.Scope != "key" {
		t.Fatal(out)
	}
	row := out.Records.Items[0]
	if row.ChargedAmount.Value.String() != "0.6" || row.ChargedAmount.Unit != "USD" || *row.Tokens.CacheWrite5m != 8 || *row.Tokens.Input != 100 || row.Details["input_cost"].String() != "0.4" {
		t.Fatal(row)
	}
	newLog := `{"id":1,"type":2,"request_id":"req-new","model_name":"m","created_at":1791885600,"quota":150,"prompt_tokens":10,"completion_tokens":3,"other":"{\"cache_tokens\":4,\"group_ratio\":0.5,\"user_group_ratio\":0.5,\"audio_input_tokens\":2,\"prompt\":\"private-prompt\",\"admin_info\":{\"secret\":\"private\"}}"}`
	for _, scope := range []string{"key", "account"} {
		path := "/api/log/token"
		body := `{"success":true,"data":[` + newLog + `,{"type":1,"content":"topup"}]}`
		if scope == "account" {
			path = "/api/log/self"
			body = `{"success":true,"data":{"total":1,"items":[` + newLog + `]}}`
		}
		out = runFixture(t, "newapi", Query{Operation: "usage-records", Scope: scope}, scope == "account", map[string]string{path: body}, nil)
		if out.Failure != "" || len(out.Records.Items) != 1 {
			t.Fatal(out)
		}
		row = out.Records.Items[0]
		if row.IDStable || row.ChargedAmount.Unit != "quota" || row.RateMultiplier.String() != "0.5" || row.Tokens.CacheWrite != nil || *row.Tokens.CacheRead != 4 {
			t.Fatal(row)
		}
		b, _ := json.Marshal(out)
		if strings.Contains(string(b), "private") {
			t.Fatal("raw metadata escaped")
		}
		if scope == "key" && (out.Records.History != "recent" || out.Records.HasMore || out.Records.Total != nil) {
			t.Fatal(out)
		}
	}
	one := `{"success":true,"data":[{"id":3,"type":2,"request_id":"req-one","model_name":"m","created_at":1791885600,"quota":60,"prompt_tokens":60,"completion_tokens":0,"content":"sensitive"}]}`
	out = runFixture(t, "oneapi", Query{Operation: "usage-records", Page: 2, PageSize: 1}, true, map[string]string{"/api/log/self": one}, func(r *http.Request) {
		if r.URL.Query().Get("p") != "1" || r.URL.Query().Get("type") != "2" {
			t.Error("wrong OneAPI pagination")
		}
	})
	if out.Failure != "" || out.Records.PageSize != 0 || out.Records.Items[0].Tokens.Semantics != "platform_fields_unverified" || out.Records.Items[0].Tokens.CacheRead != nil {
		t.Fatal(out)
	}
}
func TestFailuresBoundRequestsAndDoNotExposeUpstream(t *testing.T) {
	for _, tc := range []struct {
		status     int
		body, want string
	}{
		{401, "user-token-fixture", "authentication"}, {403, "secret", "permission_denied"}, {404, "", "unsupported"}, {429, "", "rate_limited"}, {503, "", "http_error"}, {302, "", "http_error"},
		{200, `{"success":false,"message":"private"}`, "upstream_rejected"}, {200, `<html>private</html>`, "invalid_response"}, {200, `{"success":true,"data":{}}`, "invalid_response"},
		{200, strings.Repeat("x", (2<<20)+1), "too_large"},
	} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header().Set("Location", "/unexpected")
			w.WriteHeader(tc.status)
			fmt.Fprint(w, tc.body)
		}))
		a := Adapter{HTTP: server.Client(), BaseURL: server.URL, Config: Config{Platform: "newapi", UserToken: "user-token-fixture"}}
		out := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: "balance"})
		server.Close()
		if out.Failure != tc.want || calls != 1 || out.Balance != nil {
			t.Fatalf("%s: %+v calls=%d", tc.want, out, calls)
		}
	}
	a := Adapter{Config: Config{Platform: "newapi"}}
	out := a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: "balance"})
	if out.Failure != "auth_required" || len(out.Source) != 0 {
		t.Fatal(out)
	}
	a.Config.Platform = "oneapi"
	out = a.Query(context.Background(), Query{ProviderID: "p", ExpectedRevision: 1, Operation: "effective-rate"})
	if out.Failure != "unsupported" {
		t.Fatal(out)
	}
	if !reflect.DeepEqual(a.paths, []string(nil)) {
		t.Fatal("unsupported operation made request")
	}
}

func TestPlatformSpecificEnvelopesAndCacheFields(t *testing.T) {
	for _, tc := range []struct{ platform, path, body string }{
		{"newapi", "/api/usage/token/", `{"code":false,"data":{"total_available":1}}`},
		{"newapi", "/api/usage/token/", `{"code":1,"data":{"total_available":1}}`},
		{"sub2api", "/v1/usage", `{"code":true,"mode":"unrestricted","balance":1}`},
	} {
		op := "key-quota"
		if tc.platform == "sub2api" {
			op = "balance"
		}
		out := runFixture(t, tc.platform, Query{Operation: op}, false, map[string]string{tc.path: tc.body}, nil)
		if out.Failure != "upstream_rejected" {
			t.Fatal(out)
		}
	}
	out := runFixture(t, "newapi", Query{Operation: "usage-records", Scope: "key"}, false, map[string]string{"/api/log/token": `{"success":true,"data":[{"type":2,"model_name":"m","created_at":1791885600,"quota":0,"other":"{\"cache_creation_tokens_5m\":12,\"cache_creation_tokens_1h\":3,\"audio_input\":7,\"audio_ratio\":0.5,\"cache_write_tokens\":15,\"audio_input_token_count\":8,\"tool_surcharges\":[{\"name\":\"web_search_preview\",\"count\":2,\"price\":10,\"content\":\"private\"},{\"name\":\"private-tool\",\"count\":3,\"price\":4}],\"billing_source\":\"subscription\",\"wallet_quota_deducted\":0,\"subscription_consumed\":25,\"billing_tokens\":{\"p\":90,\"cr\":15,\"secret\":33}}"}]}`}, nil)
	if out.Failure != "" {
		t.Fatal(out)
	}
	row := out.Records.Items[0]
	if *row.Tokens.CacheWrite5m != 12 || *row.Tokens.CacheWrite1h != 3 || row.Tokens.Input != nil || row.Details["audio_input"].String() != "7" || *row.Tokens.CacheWrite != 15 || row.Details["audio_input_token_count"].String() != "8" || len(row.ToolCharges) != 1 || *row.ToolCharges[0].Count != 2 || row.ToolCharges[0].Price.String() != "10" || row.BillingSource != "subscription" || row.Details["wallet_quota_deducted"].String() != "0" || row.Details["subscription_consumed"].String() != "25" || row.Details["billing_tokens.p"].String() != "90" || row.Details["billing_tokens.cr"].String() != "15" || row.ChargedAmount.Value.String() != "0" {
		t.Fatal(row)
	}
	if _, ok := row.Details["billing_tokens.secret"]; ok {
		t.Fatal("unknown billing field copied")
	}
	out = runFixture(t, "oneapi", Query{Operation: "usage-records", Scope: "key", RemoteKeyID: 8}, true, nil, nil)
	if out.Failure != "unsupported_key_filter" || len(out.Source) != 0 {
		t.Fatal(out)
	}
	out = runFixture(t, "newapi", Query{Operation: "usage-records", Scope: "key", Page: 2}, false, nil, nil)
	if out.Failure != "unsupported_pagination" || len(out.Source) != 0 {
		t.Fatal(out)
	}
}
