package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestProviderCatalogEndpoints(t *testing.T) {
	for _, tc := range []struct {
		base string
		want []catalogEndpoint
	}{
		{"https://example.com/", []catalogEndpoint{{"https://example.com/models", false}, {"https://example.com/v1/models", false}}},
		{"https://example.com/v1/", []catalogEndpoint{{"https://example.com/v1/models", false}}},
		{"https://example.com/proxy", []catalogEndpoint{{"https://example.com/proxy/models", false}, {"https://example.com/proxy/v1/models", false}}},
		{"https://example.com/proxy/v2", []catalogEndpoint{{"https://example.com/proxy/v2/models", false}}},
		{"https://example.com/proxy/v1beta1", []catalogEndpoint{{"https://example.com/proxy/v1beta1/models", false}}},
		{"https://api.openai.com/", []catalogEndpoint{{"https://api.openai.com/v1/models", false}}},
		{"https://api.anthropic.com/", []catalogEndpoint{{"https://api.anthropic.com/v1/models", false}}},
		{"https://api.moonshot.cn/", []catalogEndpoint{{"https://api.moonshot.cn/v1/models", false}}},
		{"https://api.moonshot.ai/", []catalogEndpoint{{"https://api.moonshot.ai/v1/models", false}}},
		{"https://api.siliconflow.cn/", []catalogEndpoint{{"https://api.siliconflow.cn/v1/models", false}}},
		{"https://api.siliconflow.com/", []catalogEndpoint{{"https://api.siliconflow.com/v1/models", false}}},
		{"https://api.deepseek.com", []catalogEndpoint{{"https://api.deepseek.com/models", false}}},
		{"https://openrouter.ai", []catalogEndpoint{{"https://openrouter.ai/api/v1/models", false}}},
		{"https://openrouter.ai/api/v1", []catalogEndpoint{{"https://openrouter.ai/api/v1/models", false}}},
		{"https://generativelanguage.googleapis.com", []catalogEndpoint{{"https://generativelanguage.googleapis.com/v1beta/models", true}}},
		{"https://generativelanguage.googleapis.com/v1beta/openai/", []catalogEndpoint{{"https://generativelanguage.googleapis.com/v1beta/models", true}}},
		{"https://example.com/v1beta/openai", []catalogEndpoint{{"https://example.com/v1beta/openai/models", false}}},
		{"https://example.com/v1beta", []catalogEndpoint{{"https://example.com/v1beta/models", true}}},
	} {
		if got := providerCatalogEndpoints(tc.base); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("base=%s got=%v want=%v", tc.base, got, tc.want)
		}
	}
}

func TestProviderCatalogRootFallback(t *testing.T) {
	for _, tc := range []struct {
		name, body, failure string
		status, calls       int
	}{
		{"sub2api-root", `{"data":[{"id":"a"}]}`, "", 200, 1},
		{"newapi-html", `<!doctype html><html>frontend</html>`, "", 200, 2},
		{"not-found", "", "", 404, 2},
		{"method", "", "", 405, 2},
		{"auth", "secret upstream error", "authentication", 401, 1},
		{"forbidden", "", "authentication", 403, 1},
		{"rate", "", "rate_limited", 429, 1},
		{"server", "", "http_error", 503, 1},
		{"malformed", `{"data":null}`, "invalid_response", 200, 1},
		{"redirect", "", "http_error", 302, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer fixture-secret" || r.URL.RawQuery != "" {
					t.Error("unexpected authentication or request")
				}
				if calls == 1 {
					if r.URL.Path != "/models" {
						t.Error("wrong initial path")
					}
					w.Header().Set("Location", "/redirected")
					w.WriteHeader(tc.status)
					io.WriteString(w, tc.body)
				} else {
					if r.URL.Path != "/v1/models" {
						t.Error("wrong fallback path")
					}
					io.WriteString(w, `{"data":[{"id":"a"}]}`)
				}
			}))
			defer upstream.Close()
			models, _, failure := f.core.readProviderCatalog(context.Background(), state.Provider{BaseURL: upstream.URL, Secret: "fixture-secret", Protocol: "responses"})
			if calls != tc.calls || failure != tc.failure || (failure == "" && !reflect.DeepEqual(models, []string{"a"})) || (failure != "" && len(models) != 0) {
				t.Fatalf("models=%v failure=%s calls=%d", models, failure, calls)
			}
		})
	}
}

func TestProviderCatalogPagination(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			f := setup(t)
			calls := 0
			prefix := "/v1"
			if native {
				prefix = "/v1beta"
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != prefix+"/models" {
					t.Error("wrong path")
				}
				if native {
					if r.Header.Get("x-goog-api-key") != "fixture-secret" || r.Header.Get("Authorization") != "" {
						t.Error("wrong Google auth")
					}
					if calls == 1 {
						io.WriteString(w, `{"models":[{"name":"models/z"}],"nextPageToken":"cursor&injected=1"}`)
					} else {
						if r.URL.Query().Get("pageToken") != "cursor&injected=1" || r.URL.Query().Get("pageSize") != "1000" || len(r.URL.Query()) != 2 {
							t.Error("unsafe cursor encoding")
						}
						io.WriteString(w, `{"models":[{"name":"models/a"},{"name":"models/z"}]}`)
					}
				} else {
					if r.Header.Get("x-api-key") != "fixture-secret" || r.Header.Get("anthropic-version") != "2023-06-01" {
						t.Error("wrong Anthropic auth")
					}
					if calls == 1 {
						io.WriteString(w, `{"data":[{"id":"z"}],"has_more":true,"last_id":"z"}`)
					} else {
						if r.URL.Query().Get("after_id") != "z" || r.URL.Query().Get("limit") != "1000" || len(r.URL.Query()) != 2 {
							t.Error("wrong cursor")
						}
						io.WriteString(w, `{"data":[{"id":"a"},{"id":"z"}],"has_more":false}`)
					}
				}
			}))
			defer upstream.Close()
			models, status, failure := f.core.readProviderCatalog(context.Background(), state.Provider{BaseURL: upstream.URL + prefix, Secret: "fixture-secret", Protocol: "anthropic"})
			if status != 200 || failure != "" || calls != 2 || !reflect.DeepEqual(models, []string{"a", "z"}) {
				t.Fatalf("models=%v status=%d failure=%s calls=%d", models, status, failure, calls)
			}
		})
	}
}

func TestProviderCatalogPartialFailureAndLimits(t *testing.T) {
	for _, tc := range []struct {
		name, failure string
		calls         int
	}{
		{"second-auth", "authentication", 2}, {"second-invalid", "invalid_response", 2},
		{"cycle", "invalid_response", 2}, {"format-change", "invalid_response", 2},
		{"rows", "too_large", 2}, {"bytes", "too_large", 2}, {"pages", "too_large", maxCatalogRequests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 2 {
					switch tc.name {
					case "second-auth":
						w.WriteHeader(401)
						return
					case "second-invalid":
						io.WriteString(w, `{"data":null}`)
						return
					case "format-change":
						io.WriteString(w, `{"models":[{"name":"models/a"}]}`)
						return
					}
				}
				cursor := fmt.Sprintf("page-%d", calls)
				if tc.name == "cycle" {
					cursor = "same"
				}
				rows := `{"id":"` + cursor + `"}`
				if tc.name == "rows" {
					rows = strings.Repeat(`{"id":"duplicate"},`, 500) + rows
				}
				padding := ""
				if tc.name == "bytes" {
					padding = strings.Repeat("x", catalog.MaxBytes/2)
				}
				fmt.Fprintf(w, `{"data":[%s],"has_more":true,"last_id":%q,"padding":%q}`, rows, cursor, padding)
			}))
			defer upstream.Close()
			models, _, failure := f.core.readProviderCatalog(context.Background(), state.Provider{BaseURL: upstream.URL + "/v1"})
			if models == nil || len(models) != 0 || failure != tc.failure || calls != tc.calls {
				t.Fatalf("models=%v failure=%s calls=%d", models, failure, calls)
			}
		})
	}
}

func TestProviderCatalogSharedTimeout(t *testing.T) {
	f := setup(t)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			io.WriteString(w, `{"data":[{"id":"a"}],"has_more":true,"last_id":"a"}`)
		} else {
			<-r.Context().Done()
		}
	}))
	defer upstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	models, _, failure := f.core.readProviderCatalog(ctx, state.Provider{BaseURL: upstream.URL + "/v1"})
	if failure != "timeout" || len(models) != 0 || calls != 2 {
		t.Fatalf("failure=%s calls=%d", failure, calls)
	}
}
