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

func TestCatalogRateResolvesExactSub2Key(t *testing.T) {
	for _, personal := range []bool{true, false} {
		t.Run(fmt.Sprint(personal), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer account-rate-fixture" {
					t.Error("unexpected auth/verb")
				}
				switch r.URL.Path {
				case "/prefix/api/v1/keys":
					if r.URL.Query().Get("page") != "1" || r.URL.Query().Get("page_size") != "100" {
						t.Error("unbounded list")
					}
					fmt.Fprint(w, `{"code":0,"data":{"items":[{"id":1,"key":"other-rate-fixture"},{"id":7,"key":"model-rate-fixture"}],"total":2}}`)
				case "/prefix/api/v1/keys/7":
					fmt.Fprint(w, `{"code":0,"data":{"group_id":3,"group":{"name":"Pro"}}}`)
				case "/prefix/api/v1/groups/rates":
					if personal {
						fmt.Fprint(w, `{"code":0,"data":{"3":0.8123456789012345}}`)
					} else {
						fmt.Fprint(w, `{"code":0,"data":null}`)
					}
				case "/prefix/api/v1/groups/available":
					fmt.Fprint(w, `{"code":0,"data":[{"id":3,"name":"Pro","rate_multiplier":0}]}`)
				default:
					t.Error("wrong key", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			a := Adapter{HTTP: server.Client(), BaseURL: server.URL + "/prefix/v1", Config: Config{Platform: "sub2api", UserToken: "account-rate-fixture"}, Key: "model-rate-fixture"}
			out := a.QueryKeyRate(context.Background())
			if out.Failure != "" || out.EffectiveRate == nil || out.EffectiveRate.Group != "3" || out.EffectiveRate.GroupName != "Pro" {
				t.Fatal(out)
			}
			want, basis := "0", "authenticated_group"
			if personal {
				want, basis = "0.8123456789012345", "personal_override"
			}
			if out.EffectiveRate.Multiplier.String() != want || out.EffectiveRate.Basis != basis {
				t.Fatal(out)
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), a.Key) || strings.Contains(string(raw), a.Config.UserToken) {
				t.Fatal("secret leaked")
			}
		})
	}
}

func TestCatalogRateVerifiesMaskedNewAPIKey(t *testing.T) {
	for _, match := range []bool{true, false} {
		t.Run(fmt.Sprint(match), func(t *testing.T) {
			verified := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer account-rate-fixture" || r.Header.Get("New-Api-User") != "12" || strings.Contains(r.URL.String(), "abcdefghijkl") {
					t.Error("credential boundary")
				}
				switch r.URL.Path {
				case "/api/token/":
					fmt.Fprint(w, `{"success":true,"data":{"items":[{"id":8,"key":"abcd********ijkl"}],"total":1}}`)
				case "/api/token/8/key":
					if r.Method != "POST" || r.ContentLength > 0 {
						t.Error("expected fixed read-only POST")
					}
					verified = true
					key := "abcdefghijkl"
					if !match {
						key = "abcdDIFFERENTijkl"
					}
					fmt.Fprintf(w, `{"success":true,"data":{"key":%q}}`, key)
				case "/api/token/8":
					if !verified || !match {
						t.Error("unverified identity used")
					}
					fmt.Fprint(w, `{"success":true,"data":{"group":"vip"}}`)
				case "/api/pricing":
					fmt.Fprint(w, `{"success":true,"group_ratio":{"vip":1.2}}`)
				default:
					t.Error("unexpected path", r.URL.Path)
				}
			}))
			defer server.Close()
			a := Adapter{HTTP: server.Client(), BaseURL: server.URL + "/v1", Config: Config{Platform: "newapi", UserToken: "account-rate-fixture", UserID: 12}, Key: "sk-abcdefghijkl"}
			out := a.QueryKeyRate(context.Background())
			if !verified {
				t.Fatal("did not verify")
			}
			if !match {
				if out.Failure != "key_not_found" || out.EffectiveRate != nil {
					t.Fatal(out)
				}
				return
			}
			if out.Failure != "" || out.EffectiveRate == nil || out.EffectiveRate.Multiplier.String() != "1.2" {
				t.Fatal(out)
			}
		})
	}
}
func TestCatalogRateNoAuthDoesNotGuess(t *testing.T) {
	for _, tc := range []struct{ platform, want string }{{"", "platform_required"}, {"sub2api", "auth_required"}, {"newapi", "auth_required"}, {"oneapi", "unsupported"}} {
		a := Adapter{Config: Config{Platform: tc.platform}, Key: "model-rate-fixture"}
		out := a.QueryKeyRate(context.Background())
		if out.Failure != tc.want || out.EffectiveRate != nil || len(out.Source) != 0 {
			t.Fatal(out)
		}
	}
}
