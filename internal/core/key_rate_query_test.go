package core

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/providerapi"
)

func TestAutomaticRateQueryCachesOnlyMatchedLocalKey(t *testing.T) {
	f := setup(t)
	var fail atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sub2api/billing":
			if fail.Load() {
				w.WriteHeader(503)
				return
			}
			// Use the same shape as the adapter's Key billing fixtures.
			io.WriteString(w, `{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":2,"resolved_rate_multiplier":2,"peak_rate_enabled":false,"effective_rate_multiplier":2}`)
		case "/api/v1/keys/11":
			io.WriteString(w, `{"code":0,"data":{"group_id":1,"group":{"name":"Remote"}}}`)
		case "/api/v1/groups/rates":
			io.WriteString(w, `{"code":0,"data":{"1":9}}`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	f.add(t, "p", upstream.URL+"/v1", "rate-query-fixture")
	token := "rate-account-fixture"
	settings, err := f.store.ConfigureProviderPlatform(providerapi.Configure{ProviderID: "p", ExpectedRevision: 1, Config: providerapi.Config{Platform: "sub2api"}, UserToken: &token})
	if err != nil {
		t.Fatal(err)
	}
	q := providerapi.Query{ProviderID: "p", ExpectedRevision: settings.Revision, Operation: "effective-rate"}
	out, err := f.core.QueryProvider(context.Background(), q)
	if err != nil || out.Failure != "" || out.EffectiveRate == nil {
		t.Fatalf("%+v %v", out, err)
	}
	keys, _ := f.store.ProviderKeys("p")
	if keys[0].Rate == nil || keys[0].Rate.Multiplier == nil || *keys[0].Rate.Multiplier != 2 {
		t.Fatalf("cache: %+v", keys[0].Rate)
	}
	observed := keys[0].Rate.ObservedAt
	q.RemoteKeyID = 11
	out, err = f.core.QueryProvider(context.Background(), q)
	if err != nil || out.Failure != "" || out.EffectiveRate == nil || out.EffectiveRate.Multiplier.String() != "9" {
		t.Fatalf("remote query: %+v %v", out, err)
	}
	keys, _ = f.store.ProviderKeys("p")
	if *keys[0].Rate.Multiplier != 2 || keys[0].Rate.ObservedAt != observed {
		t.Fatal("remote ID contaminated cache")
	}
	q.RemoteKeyID = 0
	fail.Store(true)
	out, err = f.core.QueryProvider(context.Background(), q)
	if err != nil || out.Failure == "" {
		t.Fatalf("failure: %+v %v", out, err)
	}
	keys, _ = f.store.ProviderKeys("p")
	if *keys[0].Rate.Multiplier != 2 || keys[0].Rate.ObservedAt != observed || keys[0].Rate.Failure == "" {
		t.Fatal("failed refresh lost cache", keys[0].Rate)
	}
}
