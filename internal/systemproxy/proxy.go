// Package systemproxy resolves explicit environment and OS proxy settings.
package systemproxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpproxy"
)

var ErrInvalid = errors.New("system proxy configuration or discovery failed")

type Reader func(context.Context, *url.URL) (*url.URL, error)

func ForRequest(r *http.Request) (*url.URL, error) {
	return Resolve(r, os.Getenv, ReadSystem)
}

// Read fresh settings for every request (including redirects). net/http's
// ProxyFromEnvironment caches the first environment snapshot for the process.
func Resolve(r *http.Request, lookupEnv func(string) string, systemProxy Reader) (*url.URL, error) {
	if LocalTarget(r.URL) {
		return nil, nil
	}
	get := func(upper, lower string) string {
		if v := lookupEnv(upper); v != "" {
			return v
		}
		return lookupEnv(lower)
	}
	selected := ""
	if r.URL.Scheme == "https" {
		selected = get("HTTPS_PROXY", "https_proxy")
	}
	if r.URL.Scheme == "http" {
		selected = get("HTTP_PROXY", "http_proxy")
	}
	if selected == "" {
		selected = get("ALL_PROXY", "all_proxy")
	}
	bypass := get("NO_PROXY", "no_proxy")
	// Always honour NO_PROXY, including when the proxy comes from Windows.
	probe := &httpproxy.Config{HTTPProxy: "http://proxy.invalid", HTTPSProxy: "http://proxy.invalid", NoProxy: bypass}
	if u, err := probe.ProxyFunc()(r.URL); err != nil {
		return nil, err
	} else if u == nil {
		return nil, nil
	}
	if selected != "" {
		u, err := Parse(selected)
		if err != nil {
			return nil, err
		}
		cfg := &httpproxy.Config{HTTPProxy: u.String(), HTTPSProxy: u.String(), NoProxy: bypass, CGI: lookupEnv("REQUEST_METHOD") != ""}
		return cfg.ProxyFunc()(r.URL)
	}
	return systemProxy(r.Context(), r.URL)
}

func LocalTarget(target *url.URL) bool {
	host := target.Hostname()
	ip := net.ParseIP(host)
	return strings.EqualFold(host, "localhost") || ip != nil && ip.IsLoopback()
}

func Parse(value string) (*url.URL, error) {
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, ErrInvalid
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, ErrInvalid
		}
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, ErrInvalid
	}
	return u, nil
}

// Windows accepts semicolon or whitespace separated proxy/bypass entries.
func Pick(server, override string, target *url.URL) (*url.URL, error) {
	if LocalTarget(target) {
		return nil, nil
	}
	host := strings.ToLower(target.Hostname())
	split := func(s string) []string {
		return strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ' ' || r == '\t' || r == '\n' })
	}
	for _, rule := range split(override) {
		rule = strings.ToLower(rule)
		if rule == "<local>" {
			if !strings.Contains(host, ".") {
				return nil, nil
			}
			continue
		}
		if ok, _ := path.Match(rule, host); ok {
			return nil, nil
		}
	}
	entries := map[string]string{}
	for _, part := range split(server) {
		if k, v, found := strings.Cut(part, "="); found {
			if _, ok := entries[strings.ToLower(k)]; !ok {
				entries[strings.ToLower(k)] = v
			}
		} else if entries[""] == "" {
			entries[""] = part
		}
	}
	value := entries[target.Scheme]
	if value == "" {
		value = entries[""]
	}
	if value == "" && entries["socks"] != "" {
		value = entries["socks"]
		if !strings.Contains(value, "://") {
			value = "socks5://" + value
		}
	}
	if value == "" {
		return nil, nil
	}
	return Parse(value)
}
