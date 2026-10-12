package state

import (
	"reflect"
	"strings"
	"testing"
)

func TestProviderMultipleURLsHomepageAndLegacyProtocol(t *testing.T) {
	s := newStore(t)
	p := provider("multi")
	p.BaseURL, p.Protocol = "", ""
	p.HomepageURL = "https://www.example.invalid"
	p.BaseURLs = []string{"https://a.example.invalid/v1", "https://b.example.invalid/v1"}
	saved, err := s.PutProvider(p, 0, nil)
	if err != nil || saved.BaseURL != p.BaseURLs[0] || saved.Protocol != "responses" {
		t.Fatal("create", err, saved.BaseURL, saved.Protocol)
	}
	got, _ := s.Provider("multi")
	if !reflect.DeepEqual(got.URLs(), p.BaseURLs) || got.HomepageURL != p.HomepageURL {
		t.Fatal("readback", got)
	}
	// An empty protocol on update keeps the stored legacy value.
	if _, err = s.db.Exec(`UPDATE providers SET document=replace(document,'"protocol":"responses"','"protocol":"anthropic"') WHERE id='multi'`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PutProvider(p, 1, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.Provider("multi"); got.Protocol != "anthropic" {
		t.Fatal("protocol not preserved", got.Protocol)
	}
	// Copy preserves the URL list and homepage.
	if err = s.MutateProvider("multi", 2, "copy", "multi-copy", "", "", false, false); err != nil {
		t.Fatal(err)
	}
	if cp, _ := s.Provider("multi-copy"); !reflect.DeepEqual(cp.URLs(), p.BaseURLs) || cp.HomepageURL != p.HomepageURL {
		t.Fatal("copy lost URLs", cp)
	}
}

func TestLegacySingleBaseURLDocumentIsRead(t *testing.T) {
	s := newStore(t)
	if _, err := s.db.Exec(`INSERT INTO providers(id,document,secret,revision) VALUES('old','{"id":"old","base_url":"https://old.example.invalid/v1","protocol":"chat-completions","enabled":true,"timeout_seconds":10,"max_concurrent":2}','',1)`); err != nil {
		t.Fatal(err)
	}
	got, err := s.Provider("old")
	if err != nil || !reflect.DeepEqual(got.BaseURLs, []string{"https://old.example.invalid/v1"}) || got.Protocol != "chat-completions" {
		t.Fatal("legacy read", err, got)
	}
}

func TestProviderURLValidation(t *testing.T) {
	ok := provider("v")
	ok.BaseURL = ""
	cases := map[string]func(*Provider){
		"none": func(p *Provider) { p.BaseURLs = nil },
		"duplicate": func(p *Provider) {
			p.BaseURLs = []string{"https://a.example.invalid/v1", "https://a.example.invalid/v1/"}
		},
		"too many": func(p *Provider) {
			for i := 0; i <= MaxProviderURLs; i++ {
				p.BaseURLs = append(p.BaseURLs, "https://a"+strings.Repeat("a", i)+".example.invalid")
			}
		},
		"mirror":   func(p *Provider) { p.BaseURL = "https://other.example.invalid" },
		"homepage": func(p *Provider) { p.HomepageURL = "ftp://example.invalid" },
		"userinfo": func(p *Provider) { p.HomepageURL = "https://u:p@example.invalid" },
	}
	for name, mutate := range cases {
		p := ok
		p.BaseURLs = []string{"https://a.example.invalid/v1"}
		mutate(&p)
		if p.Validate() == nil {
			t.Error(name, "accepted")
		}
	}
	ok.BaseURLs = []string{"https://a.example.invalid/v1", "https://b.example.invalid/v1"}
	ok.Protocol = ""
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
}
