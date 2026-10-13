package providerapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

type object map[string]json.RawMessage

// get never follows redirects or returns upstream error bodies. One operation
// shares an eight request / 2 MiB budget and its caller's deadline.
func (a *Adapter) get(ctx context.Context, path string, user bool, query url.Values) (object, error) {
	return a.getWithAuth(ctx, path, user, false, query)
}

func (a *Adapter) getWithAuth(ctx context.Context, path string, user, public bool, query url.Values) (object, error) {
	return a.read(ctx, http.MethodGet, path, user, public, query)
}

// read is internal; POST is used only for NewAPI's read-only full-key lookup.
func (a *Adapter) read(ctx context.Context, method, path string, user, public bool, query url.Values) (object, error) {
	a.calls++
	if a.calls > 8 {
		return nil, failure("too_large")
	}
	secret := a.Key
	if user {
		secret = a.Config.UserToken
	}
	if public {
		secret = ""
	}
	u, err := url.Parse(a.root() + path)
	if err != nil {
		return nil, failure("invalid_request")
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, failure("invalid_request")
	}
	req.Header.Set("Accept", "application/json")
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	if user && a.Config.Platform == "newapi" && a.Config.UserID > 0 {
		req.Header.Set("New-Api-User", remoteID(a.Config.UserID))
	}
	a.paths = append(a.paths, path)
	client := http.Client{}
	if a.HTTP != nil {
		client = *a.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, failure("timeout")
		}
		return nil, failure("network")
	}
	defer resp.Body.Close()
	a.status = resp.StatusCode
	if resp.StatusCode != 200 {
		code := "http_error"
		switch resp.StatusCode {
		case 401:
			code = "authentication"
		case 403:
			code = "permission_denied"
		case 404, 405:
			code = "unsupported"
		case 429:
			code = "rate_limited"
		}
		return nil, failure(code)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64((2<<20)-a.bytes+1)))
	a.bytes += len(raw)
	if a.bytes > 2<<20 {
		return nil, failure("too_large")
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, failure("timeout")
		}
		return nil, failure("network")
	}
	var m object
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return nil, failure("invalid_response")
	}
	if v, ok := m["success"]; ok && string(v) != "true" {
		return nil, failure("upstream_rejected")
	}
	if v, ok := m["code"]; ok {
		// NewAPI's Key usage endpoint returns a boolean code; sub2api uses
		// numeric zero. Limit that exception to the endpoint which defines it.
		accepted := string(v) == "0"
		if path == "/api/usage/token/" {
			accepted = accepted || string(v) == "true"
		}
		if !accepted {
			return nil, failure("upstream_rejected")
		}
	}
	return m, nil
}
func data(m object) object {
	if v, ok := m["data"]; ok {
		var out object
		if json.Unmarshal(v, &out) == nil && out != nil {
			return out
		}
		return nil
	}
	return m
}
func number(m object, key string) *json.Number {
	raw := m[key]
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || raw[0] == '"' {
		return nil
	}
	var n json.Number
	if json.Unmarshal(raw, &n) != nil {
		return nil
	}
	if _, err := strconv.ParseFloat(n.String(), 64); err != nil {
		return nil
	}
	return &n
}
func integer(m object, key string) *int64 {
	n := number(m, key)
	if n == nil {
		return nil
	}
	v, e := n.Int64()
	if e != nil {
		return nil
	}
	return &v
}
func text(m object, key string) string {
	var s string
	if json.Unmarshal(m[key], &s) != nil || len(s) > 512 || !utf8.ValidString(s) || strings.ContainsAny(s, "\r\n\x00") {
		return ""
	}
	return s
}
func identifier(m object, key string) string {
	if s := text(m, key); s != "" {
		return s
	}
	if n := integer(m, key); n != nil {
		return remoteID(*n)
	}
	return ""
}
func boolean(m object, key string) *bool {
	var b bool
	v, ok := m[key]
	if !ok || string(v) == "null" || json.Unmarshal(v, &b) != nil {
		return nil
	}
	return &b
}
func amount(m object, key, unit string) *Amount {
	if n := number(m, key); n != nil {
		return &Amount{*n, unit}
	}
	return nil
}
func array(raw json.RawMessage) ([]object, error) {
	var rows []object
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &rows) != nil || len(rows) > 1000 {
		return nil, failure("invalid_response")
	}
	for _, r := range rows {
		if r == nil {
			return nil, failure("invalid_response")
		}
	}
	return rows, nil
}
func allowance(scope string) *Allowance { return &Allowance{Scope: scope, Windows: []Window{}} }
