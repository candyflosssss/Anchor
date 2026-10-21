package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/providerapi"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestPlatformTypedClientRoundTrip(t *testing.T) {
	c, _, _, _ := connection(t)
	ctx := context.Background()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/settings/public" {
			w.WriteHeader(404)
			return
		}
		// NewAPI balance reads public display metadata without user authorization.
		// This fixture intentionally omits conversion metadata to retain raw quota.
		if r.URL.Path == "/api/status" {
			if r.Header.Get("Authorization") != "" || r.Header.Get("New-Api-User") != "" {
				t.Error("public status leaked user authorization")
			}
			io.WriteString(w, `{"success":true,"data":{}}`)
			return
		}
		if r.URL.Path != "/api/user/self" || r.Header.Get("Authorization") != "Bearer dashboard-fixture" || r.Header.Get("New-Api-User") != "9" {
			t.Error("wrong user authorization")
		}
		io.WriteString(w, `{"success":true,"data":{"quota":345,"used_quota":12}}`)
	}))
	defer up.Close()
	p, e := c.PutProvider(ctx, state.Provider{ID: "p", BaseURL: up.URL + "/v1", Protocol: "responses", Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, 0, nil)
	if e != nil {
		t.Fatal(e)
	}
	token := "dashboard-fixture"
	saved, e := c.ConfigureProviderPlatform(ctx, providerapi.Configure{ProviderID: p.ID, ExpectedRevision: p.Revision, Config: providerapi.Config{Platform: "newapi", UserID: 9}, UserToken: &token})
	if e != nil {
		t.Fatal(e)
	}
	got, e := c.ProviderPlatform(ctx, "p")
	if e != nil || got.Revision != 2 || !got.Config.HasUserToken {
		t.Fatal(got, e)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), token) {
		t.Fatal("credential returned")
	}
	out, e := c.QueryProvider(ctx, providerapi.Query{ProviderID: "p", ExpectedRevision: saved.Revision, Operation: "balance"})
	if e != nil || out.Failure != "" || out.Balance.Remaining.Value.String() != "345" || out.Balance.Remaining.Unit != "quota" {
		t.Fatal(out, e)
	}
	if _, e = c.QueryProvider(ctx, providerapi.Query{ProviderID: "p", ExpectedRevision: 1, Operation: "balance"}); e == nil {
		t.Fatal("stale revision accepted")
	}
}
