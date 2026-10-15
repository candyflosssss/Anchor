package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelSpeedFlags(t *testing.T) {
	for _, provider := range []bool{false, true} {
		t.Run(map[bool]string{false: "model", true: "provider"}[provider], func(t *testing.T) {
			writes := 0
			web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "GET" && r.URL.Path == "/control/v1/instance" {
					json.NewEncoder(w).Encode(client.Instance{InstanceID: "test", APIVersion: 1})
					return
				}
				if provider {
					var in catalog.Action
					json.NewDecoder(r.Body).Decode(&in)
					if in.ServiceTier == nil || *in.ServiceTier != "" || strings.Join(in.SpeedVariants, ",") != "fast,ultrafast" {
						t.Errorf("bad action: %+v", in)
					}
				} else {
					var in struct {
						Model state.Model `json:"model"`
					}
					json.NewDecoder(r.Body).Decode(&in)
					if in.Model.ID != "gpt-6.1-sol(fast)" || in.Model.ServiceTier != "priority" {
						t.Errorf("bad model: %+v", in)
					}
				}
				writes++
				io.WriteString(w, "{}")
			}))
			defer web.Close()
			c, _, err := client.Connect(context.Background(), state.Context{Name: "test", Endpoint: web.URL, InstanceID: "test", CredentialRef: "local-owner"}, "fixture-only")
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			a := App{Out: io.Discard, Err: io.Discard}
			if provider {
				err = a.providerFeature(context.Background(), c, []string{"model-put", "--id", "p", "--revision", "1", "--target", "gpt-6.1-sol", "--model", "gpt-6.1-sol", "--speed-variant", "fast", "--speed-variant", "ultrafast", "--service-tier="})
			} else {
				err = a.models(context.Background(), c, []string{"add", "--id", "gpt-6.1-sol(fast)", "--candidate", "p:gpt-6.1-sol", "--service-tier", "priority"})
			}
			if err != nil || writes != 1 {
				t.Fatal(writes, err)
			}
		})
	}
}

func TestSpeedHelpAndRejectInvalidTier(t *testing.T) {
	var out bytes.Buffer
	a := App{Err: &out}
	if err := a.providerFeature(context.Background(), nil, []string{"model-put", "--help"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatal(err)
	}
	for _, name := range []string{"-service-tier", "-speed-variant"} {
		if !strings.Contains(out.String(), name) {
			t.Fatal(name)
		}
	}
	if err := a.models(context.Background(), nil, []string{"add", "--id", "base", "--candidate", "p:up", "--service-tier", "fake"}); err == nil {
		t.Fatal("invalid tier accepted")
	}
}
