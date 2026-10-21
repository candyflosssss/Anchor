package state

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/providerapi"
)

func TestPlatformCredentialLifecycle(t *testing.T) {
	s := newStore(t)
	p, err := s.PutProvider(provider("p"), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	secret := "dashboard-fixture-secret"
	cfg := providerapi.Config{Platform: "newapi", UserID: 7}
	save := func(token *string) providerapi.Settings {
		t.Helper()
		out, e := s.ConfigureProviderPlatform(providerapi.Configure{ProviderID: p.ID, ExpectedRevision: p.Revision, Config: cfg, UserToken: token})
		if e != nil {
			t.Fatal(e)
		}
		p.Revision = out.Revision
		raw, _ := json.Marshal(out)
		if strings.Contains(string(raw), secret) || out.Config.UserToken != "" {
			t.Fatal("credential leaked")
		}
		return out
	}
	if !save(&secret).Config.HasUserToken {
		t.Fatal("credential not saved")
	}
	if _, e := s.ConfigureProviderPlatform(providerapi.Configure{ProviderID: p.ID, ExpectedRevision: 1, Config: cfg}); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if !save(nil).Config.HasUserToken {
		t.Fatal("same target lost credential")
	}
	cfg.UserID = 8
	if save(nil).Config.HasUserToken {
		t.Fatal("new user inherited credential")
	}
	save(&secret)
	cfg.SiteURL = "https://dashboard.invalid/proxy/"
	if save(nil).Config.HasUserToken {
		t.Fatal("new site inherited credential")
	}
	cfg.SiteURL = strings.TrimRight(cfg.SiteURL, "/")
	save(&secret)
	cfg.Platform = "sub2api"
	if save(nil).Config.HasUserToken {
		t.Fatal("new platform inherited credential")
	}
	save(&secret)
	empty := ""
	if save(&empty).Config.HasUserToken {
		t.Fatal("explicit clear failed")
	}
}

func TestPlatformDerivedRootCopyRenameDelete(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "derived", true: "explicit"}[explicit], func(t *testing.T) {
			s := newStore(t)
			p, e := s.PutProvider(provider("p"), 0, nil)
			if e != nil {
				t.Fatal(e)
			}
			token := "dashboard-fixture-secret"
			cfg := providerapi.Config{Platform: "newapi"}
			if explicit {
				cfg.SiteURL = "https://dashboard.invalid/prefix"
			}
			out, e := s.ConfigureProviderPlatform(providerapi.Configure{ProviderID: p.ID, ExpectedRevision: 1, Config: cfg, UserToken: &token})
			if e != nil {
				t.Fatal(e)
			}
			p.Revision = out.Revision
			p.BaseURL = "https://changed.invalid/v1"
			p.BaseURLs = []string{p.BaseURL}
			p, e = s.PutProvider(p, p.Revision, nil)
			if e != nil {
				t.Fatal(e)
			}
			got, e := s.ProviderPlatformSettings(p.ID)
			if e != nil || got.Config.HasUserToken != explicit {
				t.Fatal(got, e)
			}
			if e = s.MutateProvider(p.ID, p.Revision, "copy", "copy", "", "", false, true); e != nil {
				t.Fatal(e)
			}
			copy, e := s.ProviderPlatformSettings("copy")
			if e != nil || copy.Config.HasUserToken || copy.Config.Platform != "newapi" {
				t.Fatal(copy, e)
			}
			if e = s.MutateProvider(p.ID, p.Revision, "rename", "renamed", "", "", false, false); e != nil {
				t.Fatal(e)
			}
			got, e = s.ProviderPlatformSettings("renamed")
			if e != nil || got.Config.HasUserToken != explicit {
				t.Fatal(got, e)
			}
			if e = s.MutateProvider("renamed", got.Revision, "delete", "", "", "", false, false); e != nil {
				t.Fatal(e)
			}
			var n int
			if e = s.db.QueryRow(`SELECT count(*) FROM provider_platforms WHERE provider_id='renamed'`).Scan(&n); e != nil || n != 0 {
				t.Fatal(n, e)
			}
		})
	}
}

func TestPlatformSchemaFourMigration(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	identity, e := s.Initialize("127.0.0.1:17861", "127.0.0.1:17860")
	if e != nil {
		t.Fatal(e)
	}
	key := "model-fixture-secret"
	p, e := s.PutProvider(provider("p"), 0, &key)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec(`DROP TABLE provider_platforms; PRAGMA user_version=4`); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(dir, false)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	got, e := s.Settings()
	if e != nil || got.InstanceID != identity.InstanceID {
		t.Fatal(got, e)
	}
	keys, e := s.ProviderKeys(p.ID)
	if e != nil || len(keys) != 1 || keys[0].Secret != key {
		t.Fatal("keys lost", e)
	}
	settings, e := s.ProviderPlatformSettings(p.ID)
	if e != nil || settings.Config.Platform != "" || settings.Revision != p.Revision {
		t.Fatal(settings, e)
	}
	var v int
	s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
	if v != 9 {
		t.Fatal(v)
	}
}

func TestDetectionSurvivesReopenAndCredentialEdits(t *testing.T) {
	for _, failure := range []string{"", "network"} {
		t.Run("failure="+failure, func(t *testing.T) {
			dir := t.TempDir()
			s, err := Open(dir, true)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			if _, err = s.PutProvider(provider("p"), 0, nil); err != nil {
				t.Fatal(err)
			}
			token := "synthetic-dashboard-token"
			settings, err := s.ConfigureProviderPlatform(providerapi.Configure{ProviderID: "p", ExpectedRevision: 1, Config: providerapi.Config{Platform: "auto"}, UserToken: &token})
			if err != nil {
				t.Fatal(err)
			}
			platform := "sub2api"
			if failure != "" {
				platform = ""
			}
			if err = s.SaveProviderPlatformDetection("p", settings.Revision, platform, failure, false); err != nil {
				t.Fatal(err)
			}
			// The automatic cache is write-once, even when another caller has a result.
			if err = s.SaveProviderPlatformDetection("p", settings.Revision, "newapi", "", false); err != nil {
				t.Fatal(err)
			}
			s.Close()
			s, err = Open(dir, false)
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.ProviderPlatform("p")
			if err != nil || got.DetectedPlatform != platform || got.DetectionFailure != failure || got.UserToken != token {
				t.Fatal(got, err)
			}
			p, _ := s.Provider("p")
			if p.Revision != settings.Revision {
				t.Fatal("detection changed revision", p.Revision)
			}
			next := "synthetic-replacement-token"
			// Supplied observation fields are read-only and cannot overwrite the cache.
			saved, err := s.ConfigureProviderPlatform(providerapi.Configure{ProviderID: "p", ExpectedRevision: p.Revision, Config: providerapi.Config{Platform: "auto", DetectedPlatform: "oneapi"}, UserToken: &next})
			if err != nil || saved.Config.DetectedPlatform != platform || saved.Config.DetectionFailure != failure {
				t.Fatal(saved, err)
			}
			got, _ = s.ProviderPlatform("p")
			if got.UserToken != next {
				t.Fatal("credential edit failed")
			}
			if err = s.SaveProviderPlatformDetection("p", p.Revision, "oneapi", "", true); !errors.Is(err, ErrConflict) {
				t.Fatal("stale detection accepted", err)
			}
		})
	}
}
