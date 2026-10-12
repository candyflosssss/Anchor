package updater

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type updateRoundTripFunc func(*http.Request) (*http.Response, error)

func (f updateRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOfficialFeedFallback(t *testing.T) {
	for _, tc := range []string{"403", "429", "network", "missing-manifest", "foreign-artifact", "wrong-version", "foreign-tag", "empty", "invalid-api", "platform"} {
		t.Run(tc, func(t *testing.T) {
			s := New("desktop", "0.1.0-dev.desktop.19")
			s.Target = "linux-amd64"
			tag := "v0.1.0-dev.desktop.20"
			base := "https://github.com/" + Repository + "/releases/download/" + tag + "/"
			m := Manifest{SchemaVersion: 1, Product: "desktop", Version: strings.TrimPrefix(tag, "v"), Channel: "dev", Artifacts: []Artifact{{File: "setup.deb", Target: "linux-amd64", Kind: "deb", URL: base + "setup.deb", SHA256: strings.Repeat("a", 64), Bytes: 10}}}
			if tc == "platform" {
				m.Artifacts[0].Target = "windows-amd64"
				m.Artifacts[0].Kind = "nsis"
				m.Artifacts[0].File = "setup.exe"
				m.Artifacts[0].URL = base + "setup.exe"
			}
			if tc == "foreign-artifact" {
				m.Artifacts[0].URL = "https://evil.invalid/setup.deb"
			}
			if tc == "wrong-version" {
				m.Version = "0.1.0-dev.desktop.21"
			}
			feedCalls := 0
			requests := []string{}
			s.Client.Transport = updateRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests = append(requests, r.URL.String())
				status := 200
				body := ""
				switch {
				case r.URL.Host == "api.github.com":
					if tc == "network" {
						return nil, errors.New("isolated network failure")
					}
					status = 403
					if tc == "429" {
						status = 429
					}
					if tc == "invalid-api" {
						status = 200
						body = "not json"
					}
				case strings.HasSuffix(r.URL.Path, "releases.atom"):
					feedCalls++
					href := "https://github.com/" + Repository + "/releases/tag/" + tag
					if tc == "foreign-tag" {
						href = "https://github.com/other/repo/releases/tag/" + tag
					}
					body = `<feed xmlns="http://www.w3.org/2005/Atom"><entry><link rel="alternate" href="` + href + `"/></entry></feed>`
					if tc == "missing-manifest" {
						body = strings.Replace(body, "</feed>", `<entry><link href="https://github.com/`+Repository+`/releases/tag/v0.1.0-dev.desktop.21"/></entry></feed>`, 1)
					}
					if tc == "empty" {
						body = `<feed xmlns="http://www.w3.org/2005/Atom"/>`
					}
				case strings.Contains(r.URL.Path, "v0.1.0-dev.desktop.21/"):
					status = 404
				default:
					b, _ := json.Marshal(m)
					body = string(b)
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
			})
			check, err := s.Check(context.Background(), Config{Channel: "dev"})
			switch tc {
			case "403", "429", "network", "missing-manifest":
				if err != nil || !check.Available || check.Version != m.Version {
					t.Fatal(check, err, requests)
				}
			case "invalid-api":
				if !errors.Is(err, ErrInvalid) || feedCalls != 0 {
					t.Fatal("invalid API must fail closed", check, err, feedCalls)
				}
			default:
				if err == nil {
					t.Fatal("invalid/empty feed must not report no update", check, requests)
				}
			}
		})
	}
}

func TestOfficialFeedLegacyCLI(t *testing.T) {
	s := New("cli", "0.1.0-alpha.5")
	s.Target = "linux-amd64"
	base := "https://github.com/" + Repository + "/releases/download/v0.1.0-alpha.6/"
	s.Client.Transport = updateRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		status := 200
		body := ""
		switch {
		case r.URL.Host == "api.github.com":
			status = 429
		case strings.HasSuffix(r.URL.Path, "releases.atom"):
			body = `<feed><entry><link href="https://github.com/` + Repository + `/releases/tag/v0.1.0-alpha.6"/></entry></feed>`
		case strings.HasSuffix(r.URL.Path, "update-cli.json"):
			status = 404
		case strings.HasSuffix(r.URL.Path, "release.json"):
			b, _ := json.Marshal(map[string]any{"schema_version": 1, "version": "0.1.0-alpha.6", "channel": "alpha", "standalone_artifacts": []Artifact{{File: "anchor", Target: "linux-amd64", SHA256: strings.Repeat("a", 64), Bytes: 10}}})
			body = string(b)
		default:
			t.Fatalf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})
	check, err := s.Check(context.Background(), Config{Channel: "alpha"})
	if err != nil || !check.Available || check.Artifact.URL != base+"anchor" || check.Artifact.Kind != "binary" {
		t.Fatal(check, err)
	}
}
