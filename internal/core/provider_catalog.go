package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/egress"
	"github.com/candyflosssss/Anchor/internal/state"
)

const maxCatalogRequests = 10

var catalogVersionPath = regexp.MustCompile(`(?:^|/)v[0-9]+(?:(?:alpha|beta)[0-9]*)?(?:/openai)?$`)

type catalogEndpoint struct {
	url    string
	google bool
}

// Preserve deployment prefixes and explicit API versions. Unversioned bases
// get a same-prefix /v1 fallback; known official providers have fixed paths.
func providerCatalogEndpoints(base string) []catalogEndpoint {
	base = strings.TrimRight(base, "/")
	u, err := url.Parse(base)
	if err != nil {
		return nil
	}
	path := strings.TrimRight(u.Path, "/")
	host := strings.ToLower(u.Hostname())
	google := host == "generativelanguage.googleapis.com"
	if strings.HasSuffix(path, "/v1beta/openai") && google {
		return []catalogEndpoint{{strings.TrimSuffix(base, "/openai") + "/models", true}}
	}
	if strings.HasSuffix(path, "/v1beta") || strings.HasSuffix(path, "/v1") && google {
		return []catalogEndpoint{{base + "/models", true}}
	}
	if path == "" {
		switch host {
		case "generativelanguage.googleapis.com":
			return []catalogEndpoint{{base + "/v1beta/models", true}}
		case "openrouter.ai":
			return []catalogEndpoint{{base + "/api/v1/models", false}}
		case "api.openai.com", "api.anthropic.com", "api.moonshot.cn", "api.moonshot.ai", "api.siliconflow.cn", "api.siliconflow.com":
			return []catalogEndpoint{{base + "/v1/models", false}}
		case "api.deepseek.com":
			return []catalogEndpoint{{base + "/models", false}}
		}
	}
	endpoints := []catalogEndpoint{{base + "/models", false}}
	if !catalogVersionPath.MatchString(path) {
		endpoints = append(endpoints, catalogEndpoint{base + "/v1/models", false})
	}
	return endpoints
}

// Every request shares one timeout, concurrency slot, byte budget and row
// budget. No inference, redirects, health updates or partial lists are allowed.
type catalogBudget struct{ bytes int }

func (s *Server) readProviderCatalog(ctx context.Context, p state.Provider) ([]string, int, string) {
	return s.readProviderCatalogBudget(ctx, p, &catalogBudget{})
}

func (s *Server) readProviderCatalogBudget(ctx context.Context, p state.Provider, budget *catalogBudget) ([]string, int, string) {
	ctx = egress.DefaultRoute(ctx, p.ID)
	endpoints := providerCatalogEndpoints(p.BaseURL)
	empty := []string{}
	if budget.bytes > catalog.MaxBytes {
		return empty, 0, "too_large"
	}
	status, requests := 0, 0
	models := []string{}
	seenCursors := map[string]bool{}
	format := ""
	for index, endpoint := range endpoints {
		if endpoint.google || p.Protocol == "anthropic" {
			u, _ := url.Parse(endpoint.url)
			query := u.Query()
			if endpoint.google {
				query.Set("pageSize", "1000")
			} else {
				query.Set("limit", "1000")
			}
			u.RawQuery = query.Encode()
			endpoint.url = u.String()
		}
		nextURL := endpoint.url
		for requests < maxCatalogRequests {
			firstPage := nextURL == endpoint.url
			requests++
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, nextURL, nil)
			if err != nil {
				return empty, status, "network"
			}
			req.Header.Set("Accept", "application/json")
			if endpoint.google {
				if p.Secret != "" {
					req.Header.Set("x-goog-api-key", p.Secret)
				}
			} else {
				authorizeUpstream(req, p, p.Secret)
			}
			resp, err := s.upstream.Do(req)
			if err != nil {
				failure := "network"
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					failure = "timeout"
				}
				return empty, status, failure
			}
			status = resp.StatusCode
			canFallback := index+1 < len(endpoints) && firstPage
			if status != http.StatusOK {
				resp.Body.Close() // Do not read or reflect error bodies.
				failure := "http_error"
				switch status {
				case 401, 403:
					failure = "authentication"
				case 404, 405:
					failure = "unsupported"
				case 429:
					failure = "rate_limited"
				}
				if (status == 404 || status == 405) && canFallback {
					break
				}
				return empty, status, failure
			}
			raw, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(catalog.MaxBytes-budget.bytes+1)))
			resp.Body.Close()
			budget.bytes += len(raw)
			if readErr != nil {
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return empty, status, "timeout"
				}
				return empty, status, "network"
			}
			if budget.bytes > catalog.MaxBytes {
				return empty, status, "too_large"
			}
			page, parseErr := catalog.ParsePage(raw, p.Secret)
			if parseErr != nil {
				// An unversioned /models can be the site's frontend HTML. Only
				// that response allows a fallback, not arbitrary broken JSON.
				if canFallback && (strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") || isCatalogHTML(raw)) {
					break
				}
				return empty, status, "invalid_response"
			}
			if format != "" && format != page.Format {
				return empty, status, "invalid_response"
			}
			format = page.Format
			models = append(models, page.Models...)
			if len(models) > catalog.MaxModels {
				return empty, status, "too_large"
			}
			if page.NextCursor == "" {
				slices.Sort(models)
				return slices.Compact(models), status, ""
			}
			if seenCursors[page.NextCursor] {
				return empty, status, "invalid_response"
			}
			seenCursors[page.NextCursor] = true
			u, _ := url.Parse(endpoint.url)
			query := u.Query()
			query.Set(page.CursorParameter, page.NextCursor)
			u.RawQuery = query.Encode()
			nextURL = u.String()
		}
		if requests == maxCatalogRequests {
			return empty, status, "too_large"
		}
	}
	return empty, status, "unsupported"
}

func isCatalogHTML(raw []byte) bool {
	prefix := bytes.ToLower(bytes.TrimSpace(raw))
	return bytes.HasPrefix(prefix, []byte("<!doctype html")) || bytes.HasPrefix(prefix, []byte("<html"))
}
