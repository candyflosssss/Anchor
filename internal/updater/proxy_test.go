package updater

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

func TestPickSystemProxy(t *testing.T) {
	gh, _ := url.Parse("https://api.github.com/repos")
	cases := []struct{ server, override, want string }{
		{"127.0.0.1:7890", "", "http://127.0.0.1:7890"},
		{"http=127.0.0.1:1;https=127.0.0.1:2", "", "http://127.0.0.1:2"},
		{"socks=127.0.0.1:1080", "", "socks5://127.0.0.1:1080"},
		{"127.0.0.1:7890", "<local>;*.github.com", ""},
		{"127.0.0.1:7890", "api.github.com", ""},
		{"ftp=127.0.0.1:1", "", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		u, err := pickSystemProxy(c.server, c.override, gh)
		got := ""
		if u != nil {
			got = u.String()
		}
		if err != nil || got != c.want {
			t.Fatalf("%q %q: got %q %v want %q", c.server, c.override, got, err, c.want)
		}
	}
	local, _ := url.Parse("http://127.0.0.1:8787/feed")
	if u, _ := pickSystemProxy("127.0.0.1:7890", "", local); u != nil {
		t.Fatal("loopback must bypass proxy", u)
	}
}

func TestProxyPrecedence(t *testing.T) {
	oldSys, oldEnv := systemProxy, lookupEnv
	defer func() { systemProxy, lookupEnv = oldSys, oldEnv }()
	systemProxy = func(_ context.Context, u *url.URL) (*url.URL, error) { return pickSystemProxy("127.0.0.1:7890", "", u) }
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(k, "")
	}
	lookupEnv = func(string) string { return "" }
	req, _ := http.NewRequest("GET", "https://api.github.com/repos", nil)
	if u, err := proxyFor(req); err != nil || u == nil || u.Host != "127.0.0.1:7890" {
		t.Fatal("system proxy not used", u, err)
	}
	// An explicit environment setting (even one bypassing this host) wins.
	lookupEnv = func(k string) string {
		if k == "HTTPS_PROXY" {
			return "http://127.0.0.1:9"
		}
		return ""
	}
	if u, err := proxyFor(req); err != nil || u == nil || u.Host != "127.0.0.1:9" {
		t.Fatal("environment must take precedence", u)
	}
}

func TestFreshSchemeSpecificEnvironment(t *testing.T) {
	oldSys, oldEnv := systemProxy, lookupEnv
	defer func() { systemProxy, lookupEnv = oldSys, oldEnv }()
	systemProxy = func(_ context.Context, u *url.URL) (*url.URL, error) { return pickSystemProxy("127.0.0.1:7890", "", u) }
	env := map[string]string{}
	lookupEnv = func(k string) string { return env[k] }
	req, _ := http.NewRequest("GET", "https://api.github.com/repos", nil)
	check := func(want string) {
		t.Helper()
		u, err := proxyFor(req)
		got := ""
		if u != nil {
			got = u.String()
		}
		if err != nil || got != want {
			t.Fatalf("got %q %v want %q", got, err, want)
		}
	}
	check("http://127.0.0.1:7890")
	env["HTTP_PROXY"] = "127.0.0.1:9" // HTTP-only must not suppress HTTPS system proxy.
	check("http://127.0.0.1:7890")
	env["ALL_PROXY"] = "socks5h://127.0.0.1:1080"
	check("socks5h://127.0.0.1:1080")
	env["HTTPS_PROXY"] = "127.0.0.1:10"
	check("http://127.0.0.1:10")
	env["HTTPS_PROXY"] = "127.0.0.1:11"
	check("http://127.0.0.1:11")
	env["NO_PROXY"] = "api.github.com"
	check("")
	delete(env, "HTTPS_PROXY")
	delete(env, "ALL_PROXY")
	check("") // NO_PROXY also bypasses system proxy.
	delete(env, "NO_PROXY")
	env["all_proxy"] = "http://127.0.0.1:12"
	check("http://127.0.0.1:12")
	env["HTTPS_PROXY"] = "file:///secret"
	if _, err := proxyFor(req); err == nil {
		t.Fatal("malformed explicit proxy must fail")
	}
	req, _ = http.NewRequest("GET", "https://127.0.0.1/feed", nil)
	check("")
}
