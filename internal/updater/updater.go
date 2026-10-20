// Package updater is the shared local-only CLI/desktop update boundary. It never
// requests a model upstream, changes the database, or kills another process.
package updater

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid update metadata or options")
	ErrIntegrity   = errors.New("update checksum or size mismatch")
	ErrUnavailable = errors.New("no compatible update")
	ErrBusy        = errors.New("update or core is already in use")
)

const Repository = "candyflosssss/Anchor"
const maxMetadata = 2 << 20
const MaxArtifact = 256 << 20

type Config struct {
	Channel     string `json:"channel"`
	ManifestURL string `json:"manifest_url"`
}
type Artifact struct {
	File    string    `json:"file"`
	Target  string    `json:"target"`
	Kind    string    `json:"kind"`
	URL     string    `json:"url"`
	SHA256  string    `json:"sha256"`
	Bytes   int64     `json:"bytes"`
	Runtime *Artifact `json:"runtime,omitempty"`
}
type Manifest struct {
	SchemaVersion int        `json:"schema_version"`
	Product       string     `json:"product"`
	Version       string     `json:"version"`
	Channel       string     `json:"channel"`
	Artifacts     []Artifact `json:"artifacts"`
}
type Check struct {
	CurrentVersion string    `json:"current_version"`
	Version        string    `json:"version"`
	Channel        string    `json:"channel"`
	Product        string    `json:"product"`
	Source         string    `json:"source"`
	Available      bool      `json:"available"`
	Artifact       *Artifact `json:"artifact"`
}
type Service struct {
	Product, Version, Target string
	// Endpoint/Client are host-only injection points for isolated tests, not JS.
	Endpoint     string
	FeedEndpoint string // host-only test seam; empty uses the official feed
	Client       *http.Client
}

// newTransport reads current environment/system proxy settings for every host.
func newTransport() *http.Transport {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = proxyFor
	return tr
}

func New(product, version string) *Service {
	return &Service{Product: product, Version: version, Target: runtime.GOOS + "-" + runtime.GOARCH, Endpoint: "https://api.github.com/repos/" + Repository + "/releases", Client: &http.Client{Timeout: 5 * time.Minute, Transport: newTransport(), CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return ErrInvalid
		}
		u := *r.URL
		u.RawQuery = ""
		if len(via) > 0 && via[0].URL.Scheme == "https" && u.Scheme != "https" {
			return ErrInvalid
		}
		return ValidateURL(u.String())
	}}}
}

// TLS is mandatory except literal loopback addresses for isolated/local dev.
// URLs cannot carry credentials or tokens; source authentication is not added.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ErrInvalid
	}
	if u.Scheme == "https" {
		return nil
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme == "http" && ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("%w: use HTTPS (HTTP allowed only on literal loopback)", ErrInvalid)
}
func (s *Service) get(ctx context.Context, raw string) (*http.Response, error) {
	// GitHub API pagination uses a generated query; all external URLs are validated
	// before reaching here. Redirects may have provider-generated signed queries.
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Anchor-Updater/"+s.Version)
	resp, err := s.Client.Do(req)
	if err != nil {
		if errors.Is(err, errProxy) {
			return nil, errProxy
		}
		return nil, errors.Join(errors.New("update request failed (network, timeout, or redirect policy)"), safeRequestCause(err))
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, &httpStatusError{code: resp.StatusCode}
	}
	return resp, nil
}
func (s *Service) json(ctx context.Context, raw string, v any) error {
	resp, err := s.get(ctx, raw)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadata+1))
	if err != nil {
		return err
	}
	if len(b) > maxMetadata {
		return ErrInvalid
	}
	if err := json.Unmarshal(b, v); err != nil {
		return ErrInvalid
	}
	return nil
}
func validateArtifact(a Artifact, product string) error {
	hash, err := hex.DecodeString(a.SHA256)
	if err != nil || len(hash) != 32 || a.SHA256 != strings.ToLower(a.SHA256) || a.Bytes < 1 || a.Bytes > MaxArtifact {
		return ErrInvalid
	}
	if a.File == "" || filepath.Base(a.File) != a.File || strings.ContainsAny(a.File, "/\\:\x00") || a.File == "." || a.File == ".." {
		return ErrInvalid
	}
	if product == "cli" && a.Kind != "binary" {
		return ErrInvalid
	}
	if product == "desktop" && a.Kind != "runtime" && !((a.Target == "windows-amd64" && a.Kind == "nsis" && strings.HasSuffix(a.File, ".exe")) || (a.Target == "linux-amd64" && a.Kind == "deb" && strings.HasSuffix(a.File, ".deb"))) {
		return ErrInvalid
	}
	if a.Kind == "runtime" && (product != "desktop" || (a.Target != "linux-amd64" && a.Target != "windows-amd64") || (a.Target == "windows-amd64" && !strings.HasSuffix(a.File, ".exe"))) {
		return ErrInvalid
	}
	if a.Runtime != nil {
		if product != "desktop" || a.Kind == "runtime" || a.Runtime.Kind != "runtime" || a.Runtime.Target != a.Target || a.Runtime.Runtime != nil {
			return ErrInvalid
		}
		if err := validateArtifact(*a.Runtime, product); err != nil {
			return err
		}
	}
	return ValidateURL(a.URL)
}
func (s *Service) validate(m Manifest, c Config) (*Artifact, error) {
	ch, err := versionChannel(m.Version)
	if err != nil || m.SchemaVersion != 1 || m.Product != s.Product || m.Channel != c.Channel || ch != m.Channel {
		return nil, ErrInvalid
	}
	var selected *Artifact
	seen := make(map[string]bool)
	for _, a := range m.Artifacts {
		if seen[a.Target] {
			return nil, ErrInvalid
		}
		seen[a.Target] = true
		if err := validateArtifact(a, s.Product); err != nil {
			return nil, err
		}
		if a.Target == s.Target {
			if selected != nil {
				return nil, ErrInvalid
			}
			copy := a
			if a.Runtime != nil {
				copy = *a.Runtime
			}
			selected = &copy
		}
	}
	if selected == nil {
		return nil, ErrUnavailable
	}
	return selected, nil
}
func (s *Service) Check(ctx context.Context, c Config) (Check, error) {
	result := Check{CurrentVersion: s.Version, Channel: c.Channel, Product: s.Product, Source: c.ManifestURL}
	if !ValidChannel(c.Channel) || (s.Product != "cli" && s.Product != "desktop") {
		return result, ErrInvalid
	}
	if _, err := parseVersion(s.Version); err != nil {
		return result, err
	}
	var m Manifest
	if c.ManifestURL != "" {
		if err := ValidateURL(c.ManifestURL); err != nil {
			return result, err
		}
		if err := s.json(ctx, c.ManifestURL, &m); err != nil {
			return result, err
		}
	} else {
		result.Source = "https://github.com/" + Repository + "/releases"
		var err error
		m, err = s.github(ctx, c)
		if err == ErrUnavailable {
			return result, nil
		}
		if err != nil {
			return result, err
		}
	}
	a, err := s.validate(m, c)
	if err != nil {
		return result, err
	}
	order, err := compareVersions(m.Version, s.Version)
	if err != nil {
		return result, err
	}
	result.Version, result.Artifact, result.Available = m.Version, a, order > 0
	return result, nil
}

type release struct {
	Tag    string `json:"tag_name"`
	Draft  bool   `json:"draft"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (s *Service) github(ctx context.Context, c Config) (Manifest, error) {
	apiCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	m, err := s.githubAPI(apiCtx, c)
	cancel()
	if err == nil || errors.Is(err, ErrUnavailable) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrIntegrity) || ctx.Err() != nil {
		return m, err
	}
	// Custom test API endpoints cannot cause an unexpected real network fallback.
	if s.Endpoint != "https://api.github.com/repos/"+Repository+"/releases" && s.FeedEndpoint == "" {
		return m, err
	}
	feed, feedErr := s.githubFeed(ctx, c)
	if feedErr != nil {
		return Manifest{}, errors.Join(err, feedErr)
	}
	return feed, nil
}

func (s *Service) githubAPI(ctx context.Context, c Config) (Manifest, error) {
	var chosen *release
	var chosenURL string
	// Do not use /latest: GitHub excludes prereleases there. Compare SemVer rather
	// than publication dates. Bound pagination to protect resources/rate limits.
	for page := 1; page <= 5; page++ {
		var rows []release
		if err := s.json(ctx, fmt.Sprintf("%s?per_page=100&page=%d", s.Endpoint, page), &rows); err != nil {
			return Manifest{}, err
		}
		for _, r := range rows {
			v := strings.TrimPrefix(r.Tag, "v")
			ch, err := versionChannel(v)
			if err != nil || r.Draft || ch != c.Channel {
				continue
			}
			assetURL := ""
			for _, a := range r.Assets {
				if a.Name == "update-"+s.Product+".json" {
					assetURL = a.URL
					break
				}
			}
			// Backwards compatibility with the already-published CLI alpha.6 manifest.
			if assetURL == "" && s.Product == "cli" {
				for _, a := range r.Assets {
					if a.Name == "release.json" {
						assetURL = a.URL
						break
					}
				}
			}
			if assetURL == "" {
				continue
			}
			if chosen != nil {
				cmp, _ := compareVersions(v, strings.TrimPrefix(chosen.Tag, "v"))
				if cmp <= 0 {
					continue
				}
			}
			copy := r
			chosen = &copy
			chosenURL = assetURL
		}
		if len(rows) < 100 {
			break
		}
	}
	if chosen == nil {
		return Manifest{}, ErrUnavailable
	}
	if err := ValidateURL(chosenURL); err != nil {
		return Manifest{}, err
	}
	var raw struct {
		Manifest
		Standalone []Artifact `json:"standalone_artifacts"`
	}
	if err := s.json(ctx, chosenURL, &raw); err != nil {
		return Manifest{}, err
	}
	m := raw.Manifest
	if m.Product == "" && s.Product == "cli" {
		m.Product = "cli"
		m.Artifacts = raw.Standalone
		for i := range m.Artifacts {
			a := &m.Artifacts[i]
			a.Kind = "binary"
			for _, asset := range chosen.Assets {
				if asset.Name == a.File {
					a.URL = asset.URL
					break
				}
			}
		}
	}
	if m.Version != strings.TrimPrefix(chosen.Tag, "v") {
		return Manifest{}, ErrInvalid
	}
	// Default source cannot redirect downloads to an unrelated author-owned URL.
	for _, a := range allArtifacts(m.Artifacts) {
		found := false
		for _, asset := range chosen.Assets {
			if asset.Name == a.File && asset.URL == a.URL {
				found = true
				break
			}
		}
		if !found {
			return Manifest{}, ErrInvalid
		}
	}
	return m, nil
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return fmt.Sprintf("update server HTTP %d", e.code) }
func safeRequestCause(err error) error {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, ErrInvalid} {
		if errors.Is(err, cause) {
			return cause
		}
	}
	return nil
}

// Both installer and optional runtime must belong to the same trusted release.
func allArtifacts(artifacts []Artifact) []Artifact {
	var out []Artifact
	for _, a := range artifacts {
		out = append(out, a)
		if a.Runtime != nil {
			out = append(out, *a.Runtime)
		}
	}
	return out
}
