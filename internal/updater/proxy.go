package updater

import (
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// systemProxy is the OS proxy configuration (Windows Internet Settings). It is
// only consulted when no proxy environment variable is set, so CLI/explicit
// settings keep precedence. PAC scripts are not evaluated.
var systemProxy = readSystemProxy

func proxyFor(r *http.Request) (*url.URL, error) {
	if u, err := http.ProxyFromEnvironment(r); u != nil || err != nil {
		return u, err
	}
	if envProxySet() {
		return nil, nil // environment explicitly bypasses this host (NO_PROXY)
	}
	server, override, ok := systemProxy()
	if !ok {
		return nil, nil
	}
	return pickSystemProxy(server, override, r.URL)
}

func envProxySet() bool {
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if lookupEnv(k) != "" {
			return true
		}
	}
	return false
}

// pickSystemProxy parses "host:port" or "http=h:p;https=h:p;socks=h:p" plus the
// semicolon bypass list ("<local>", "*.corp", "127.*").
func pickSystemProxy(server, override string, target *url.URL) (*url.URL, error) {
	host := strings.ToLower(target.Hostname())
	for _, rule := range strings.Split(override, ";") {
		rule = strings.ToLower(strings.TrimSpace(rule))
		if rule == "" {
			continue
		}
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
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() || host == "localhost" {
		return nil, nil
	}
	entries := map[string]string{}
	for _, part := range strings.Split(server, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if k, v, found := strings.Cut(part, "="); found {
			entries[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		} else {
			entries[""] = part
		}
	}
	scheme := "http"
	value := entries[target.Scheme]
	if value == "" {
		value = entries[""]
	}
	if value == "" && entries["socks"] != "" {
		value, scheme = entries["socks"], "socks5"
	}
	if value == "" {
		return nil, nil
	}
	if !strings.Contains(value, "://") {
		value = scheme + "://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return nil, nil // malformed system setting: fall back to direct
	}
	return u, nil
}
