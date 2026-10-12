package updater

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"golang.org/x/net/http/httpproxy"
)

var systemProxy = readSystemProxy
var lookupEnv = os.Getenv
var errProxy = errors.New("update proxy configuration or discovery failed")

// Read fresh settings for every request (including redirects). net/http's
// ProxyFromEnvironment caches the first environment snapshot for the process.
func proxyFor(r *http.Request) (*url.URL, error) {
	if localTarget(r.URL) {
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
		u, err := parseProxy(selected)
		if err != nil {
			return nil, err
		}
		cfg := &httpproxy.Config{HTTPProxy: u.String(), HTTPSProxy: u.String(), NoProxy: bypass, CGI: lookupEnv("REQUEST_METHOD") != ""}
		return cfg.ProxyFunc()(r.URL)
	}
	return systemProxy(r.Context(), r.URL)
}

func localTarget(target *url.URL) bool {
	host := target.Hostname()
	ip := net.ParseIP(host)
	return strings.EqualFold(host, "localhost") || ip != nil && ip.IsLoopback()
}

func parseProxy(value string) (*url.URL, error) {
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errProxy
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, errProxy
	}
	return u, nil
}

// Windows accepts semicolon or whitespace separated proxy/bypass entries.
func pickSystemProxy(server, override string, target *url.URL) (*url.URL, error) {
	if localTarget(target) {
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
	return parseProxy(value)
}
