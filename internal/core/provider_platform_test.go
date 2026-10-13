package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/providerapi"
)

func TestPlatformQueryManagementBoundariesAndSnapshot(t *testing.T) {
	f := setup(t)
	identity, _ := f.store.Settings()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/proxy/api/usage/token/" || r.Header.Get("Authorization") != "Bearer model-fixture" {
			t.Error("incorrect target or key", r.URL.Path)
		}
		io.WriteString(w, `{"code":true,"data":{"total_available":123,"total_used":7,"total_granted":130,"unlimited_quota":false}}`)
	}))
	defer upstream.Close()
	f.add(t, "p", upstream.URL+"/proxy/v1", "model-fixture")
	cfg, e := f.store.ConfigureProviderPlatform(providerapi.Configure{ProviderID: "p", ExpectedRevision: 1, Config: providerapi.Config{Platform: "newapi"}})
	if e != nil {
		t.Fatal(e)
	}
	before, _ := f.store.Provider("p")
	body := `{"provider_id":"p","expected_revision":2,"operation":"key-quota"}`
	for _, tc := range []struct {
		token, instance, origin, body string
		status                        int
	}{
		{f.token, identity.InstanceID, "", body, 401},
		{f.owner, "wrong", "", body, 409},
		{f.owner, identity.InstanceID, "https://untrusted.invalid", body, 403},
		{f.owner, identity.InstanceID, "", strings.Replace(body, `"expected_revision":2`, `"expected_revision":1`, 1), 409},
		{f.owner, identity.InstanceID, "", strings.Replace(body, `"provider_id":"p"`, `"provider_id":"other"`, 1), 400},
	} {
		req, _ := http.NewRequest("POST", f.control.URL+"/control/v1/providers/p/query", strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("X-Anchor-Instance", tc.instance)
		req.Header.Set("Origin", tc.origin)
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("status=%d want=%d", resp.StatusCode, tc.status)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid management request reached upstream")
	}
	q := providerapi.Query{ProviderID: "p", ExpectedRevision: cfg.Revision, Operation: "key-quota"}
	out, e := f.core.QueryProvider(context.Background(), q)
	if e != nil || out.Failure != "" || out.KeyQuota.Remaining.Value.String() != "123" {
		t.Fatal(out, e)
	}
	if !f.core.take("p", 1) {
		t.Fatal("could not reserve slot")
	}
	busy, e := f.core.QueryProvider(context.Background(), q)
	f.core.release("p")
	if e != nil || busy.Failure != "busy" || calls.Load() != 1 {
		t.Fatal(busy, e)
	}
	after, _ := f.store.Provider("p")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("query mutated provider")
	}
	if strings.Contains(f.logs.String(), "model-fixture") {
		t.Fatal("key leaked to logs")
	}
}

func TestUnifiedModelsUsesCatalogFallback(t *testing.T) {
	f := setup(t)
	var calls []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/proxy/models":
			w.WriteHeader(404)
		case "/proxy/v1/models":
			io.WriteString(w, `{"data":[{"id":"available-model"}]}`)
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer up.Close()
	f.add(t, "p", up.URL+"/proxy", "model-fixture")
	out, e := f.core.QueryProvider(context.Background(), providerapi.Query{ProviderID: "p", ExpectedRevision: 1, Operation: "models"})
	if e != nil || out.Failure != "" || !reflect.DeepEqual(out.Models, []string{"available-model"}) || !reflect.DeepEqual(calls, []string{"/proxy/models", "/proxy/v1/models"}) {
		raw, _ := json.Marshal(out)
		t.Fatal(string(raw), e, calls)
	}
}

func TestDefaultKeyPreferenceAndAutomaticSummary(t *testing.T) {
	f := setup(t)
	wantSecret := "default-fixture"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prefix/api/status":
			if r.Header.Get("Authorization") != "" {
				t.Error("public probe carried credential")
			}
			io.WriteString(w, `{"success":true,"data":{"version":"1","quota_per_unit":500000,"default_use_auto_group":true,"enable_batch_update":false}}`)
		case "/prefix/api/usage/token/":
			if r.Header.Get("Authorization") != "Bearer "+wantSecret {
				t.Error("wrong default key")
			}
			io.WriteString(w, `{"code":true,"data":{"total_available":123,"total_used":7,"total_granted":130}}`)
		default:
			t.Error("unexpected endpoint", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	f.add(t, "p", upstream.URL+"/prefix/v1", "default-fixture")
	if err := f.store.MutateProvider("p", 1, "key-put", "a", "fallback-fixture", "", true, false); err != nil {
		t.Fatal(err)
	}
	q := providerapi.Query{ProviderID: "p", ExpectedRevision: 2, Operation: "balance-summary"}
	check := func(want string) {
		t.Helper()
		v, e := f.core.QueryProvider(context.Background(), q)
		if e != nil || v.Failure != "" || v.KeyID != want || v.Platform != "newapi" || v.Balance.Scope != "key" {
			t.Fatalf("%+v %v", v, e)
		}
	}
	check("default")
	wantSecret = "fallback-fixture"
	q.KeyID = "a"
	check("a")
	if err := f.store.MutateProvider("p", 2, "key-put", "default", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	q.KeyID = ""
	q.ExpectedRevision = 3
	check("a")
	after, _ := f.store.Provider("p")
	cfg, _ := f.store.ProviderPlatform("p")
	if after.Revision != 3 || cfg.Platform != "" {
		t.Fatal("query mutated configuration")
	}
}
