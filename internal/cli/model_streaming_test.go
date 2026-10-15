package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestModelStreamingDefaults(t *testing.T) {
	commands := []struct {
		name     string
		args     []string
		provider bool
	}{
		{name: "model-add", args: []string{"add", "--id", "public", "--candidate", "p:actual"}},
		{name: "model-put", args: []string{"put", "--id", "public", "--candidate", "p:actual", "--revision", "1"}},
		{name: "provider-model-create", args: []string{"model-put", "--id", "p", "--revision", "1", "--target", "public", "--model", "actual"}, provider: true},
		{name: "provider-model-edit", args: []string{"model-put", "--id", "p", "--revision", "1", "--target", "public", "--model", "actual", "--model-revision", "1"}, provider: true},
	}
	for _, command := range commands {
		for _, setting := range []struct {
			name  string
			flags []string
			want  bool
		}{
			{name: "default", want: true},
			{name: "enabled", flags: []string{"--streaming"}, want: true},
			{name: "disabled", flags: []string{"--streaming=false"}, want: false},
		} {
			t.Run(command.name+"/"+setting.name, func(t *testing.T) {
				writes := 0
				web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodGet && r.URL.Path == "/control/v1/instance" {
						_ = json.NewEncoder(w).Encode(client.Instance{InstanceID: "test", APIVersion: 1})
						return
					}
					var streaming, tools bool
					if command.provider {
						if r.Method != http.MethodPost || r.URL.Path != "/control/v1/providers/p/action" {
							t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
							w.WriteHeader(404)
							return
						}
						var in catalog.Action
						if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
							t.Error(err)
						}
						streaming, tools = in.Streaming, in.Tools
					} else {
						if r.Method != http.MethodPut || r.URL.Path != "/control/v1/models/public" {
							t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
							w.WriteHeader(404)
							return
						}
						var in struct {
							Model state.Model `json:"model"`
						}
						if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
							t.Error(err)
						}
						streaming, tools = in.Model.Streaming, in.Model.Tools
					}
					writes++
					if streaming != setting.want {
						t.Errorf("streaming=%v, want %v", streaming, setting.want)
					}
					if tools {
						t.Error("changing streaming default must not enable tools")
					}
					_, _ = io.WriteString(w, "{}")
				}))
				defer web.Close()
				c, _, err := client.Connect(context.Background(), state.Context{Name: "test", Endpoint: web.URL, InstanceID: "test", CredentialRef: "local-owner"}, "fixture-only")
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				app := App{Out: io.Discard, Err: io.Discard}
				args := append(append([]string{}, command.args...), setting.flags...)
				if command.provider {
					err = app.providerFeature(context.Background(), c, args)
				} else {
					err = app.models(context.Background(), c, args)
				}
				if err != nil {
					t.Fatal(err)
				}
				if writes != 1 {
					t.Fatalf("writes=%d, want 1", writes)
				}
			})
		}
	}
}
