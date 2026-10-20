package systemproxy

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

func TestResolutionPrecedenceBypassAndFreshSettings(t *testing.T) {
	env := map[string]string{}
	lookup := func(k string) string { return env[k] }
	osProxy := func(context.Context, *url.URL) (*url.URL, error) { return url.Parse("http://127.0.0.1:7890") }
	req, _ := http.NewRequest("GET", "https://upstream.example/v1", nil)
	check := func(want string) {
		t.Helper()
		u, err := Resolve(req, lookup, osProxy)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if u != nil {
			got = u.String()
		}
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	}
	check("http://127.0.0.1:7890")
	env["HTTP_PROXY"] = "http://127.0.0.1:1"
	check("http://127.0.0.1:7890")
	env["all_proxy"] = "socks5h://127.0.0.1:1080"
	check("socks5h://127.0.0.1:1080")
	env["https_proxy"] = "http://user:synthetic@127.0.0.1:2"
	check("http://user:synthetic@127.0.0.1:2")
	env["HTTPS_PROXY"] = "http://127.0.0.1:3"
	check("http://127.0.0.1:3")
	env["HTTPS_PROXY"] = "http://127.0.0.1:4"
	check("http://127.0.0.1:4")
	env["NO_PROXY"] = "upstream.example"
	check("")
	delete(env, "NO_PROXY")
	env["HTTPS_PROXY"] = "file:///synthetic-secret"
	if _, err := Resolve(req, lookup, osProxy); err == nil {
		t.Fatal("invalid proxy accepted")
	}
	req, _ = http.NewRequest("GET", "http://127.0.0.1/local", nil)
	check("")
}

func TestPickOSProxyAndPACBypass(t *testing.T) {
	target, _ := url.Parse("https://upstream.example/v1")
	for _, c := range []struct{ server, bypass, want string }{
		{"http=localhost:1;https=localhost:2", "", "http://localhost:2"},
		{"socks=localhost:1080", "", "socks5://localhost:1080"},
		{"localhost:1", "*.example", ""},
		{"", "", ""},
	} {
		u, err := Pick(c.server, c.bypass, target)
		got := ""
		if u != nil {
			got = u.String()
		}
		if err != nil || got != c.want {
			t.Fatalf("%+v: %q %v", c, got, err)
		}
	}
}
