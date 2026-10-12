package updater

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"sort"
	"strings"
)

// The official Atom feed avoids GitHub API rate limits/proxy rules affecting
// api.github.com. It never changes the publisher or checksum policy.
func (s *Service) githubFeed(ctx context.Context, c Config) (Manifest, error) {
	endpoint := s.FeedEndpoint
	if endpoint == "" {
		endpoint = "https://github.com/" + Repository + "/releases.atom"
	}
	if err := ValidateURL(endpoint); err != nil {
		return Manifest{}, err
	}
	resp, err := s.get(ctx, endpoint)
	if err != nil {
		return Manifest{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadata+1))
	if err != nil {
		return Manifest{}, err
	}
	if len(data) > maxMetadata {
		return Manifest{}, ErrInvalid
	}
	var feed struct {
		XMLName xml.Name `xml:"feed"`
		Entries []struct {
			Links []struct {
				Href string `xml:"href,attr"`
				Rel  string `xml:"rel,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(data, &feed); err != nil {
		return Manifest{}, ErrInvalid
	}
	prefix := "https://github.com/" + Repository + "/releases/tag/"
	var tags []string
	seen := map[string]bool{}
	for _, entry := range feed.Entries {
		for _, link := range entry.Links {
			if link.Rel != "" && link.Rel != "alternate" {
				continue
			}
			if !strings.HasPrefix(link.Href, prefix) {
				continue
			}
			tag := strings.TrimPrefix(link.Href, prefix)
			v := strings.TrimPrefix(tag, "v")
			ch, err := versionChannel(v)
			if err != nil || ch != c.Channel || seen[tag] {
				continue
			}
			if s.Product == "desktop" && !strings.Contains(v, "-dev.desktop.") && ch == "dev" {
				continue
			}
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	sort.Slice(tags, func(i, j int) bool {
		cmp, _ := compareVersions(strings.TrimPrefix(tags[i], "v"), strings.TrimPrefix(tags[j], "v"))
		return cmp > 0
	})
	for i, tag := range tags {
		if i >= 5 {
			break
		}
		base := "https://github.com/" + Repository + "/releases/download/" + url.PathEscape(tag) + "/"
		var raw struct {
			Manifest
			Standalone []Artifact `json:"standalone_artifacts"`
		}
		err := s.json(ctx, base+"update-"+s.Product+".json", &raw)
		var status *httpStatusError
		if s.Product == "cli" && errors.As(err, &status) && status.code == 404 {
			err = s.json(ctx, base+"release.json", &raw)
		}
		if errors.As(err, &status) && status.code == 404 {
			continue
		} // A release may only ship the other product.
		if err != nil {
			return Manifest{}, err
		}
		m := raw.Manifest
		if s.Product == "cli" && m.Product == "" {
			m.Product = "cli"
			m.Artifacts = raw.Standalone
			for i := range m.Artifacts {
				m.Artifacts[i].Kind = "binary"
				m.Artifacts[i].URL = base + m.Artifacts[i].File
			}
		}
		if m.Version != strings.TrimPrefix(tag, "v") {
			return Manifest{}, ErrInvalid
		}
		if _, err := s.validate(m, c); err != nil {
			return Manifest{}, err
		}
		for _, a := range m.Artifacts {
			if a.URL != base+a.File {
				return Manifest{}, ErrInvalid
			}
		}
		return m, nil
	}
	// Discovery failed rather than proving no update exists: Atom is a bounded
	// recent-release feed and is not an exhaustive catalogue of every channel.
	return Manifest{}, errors.New("official release feed has no compatible update manifest")
}
