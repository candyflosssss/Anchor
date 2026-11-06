package state

import (
	"errors"
	"testing"
)

func TestCodexSettingsDefaultPersistenceAndConflict(t *testing.T) {
	s, err := Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	out, err := s.CodexSettings()
	if err != nil || out.Retries != 5 || out.Revision != 1 {
		t.Fatal(out, err)
	}
	for _, n := range []int{0, 7, 10} {
		in := SaveCodexSettings{Retries: &n, ExpectedRevision: out.Revision}
		out, err = s.SaveCodexSettings(in)
		if err != nil || out.Retries != n || out.Revision != in.ExpectedRevision+1 {
			t.Fatal(out, err)
		}
		if _, err = s.SaveCodexSettings(in); !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	for _, n := range []int{-1, 11} {
		if _, err = s.SaveCodexSettings(SaveCodexSettings{Retries: &n, ExpectedRevision: out.Revision}); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if _, err = s.SaveCodexSettings(SaveCodexSettings{ExpectedRevision: out.Revision}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	reopened, err := Open(s.Dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.CodexSettings()
	if err != nil || got != out {
		t.Fatal(got, out, err)
	}
}

func TestCodexCompactionSettingsMigrationAndPartialUpdates(t *testing.T) {
	s, err := Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.db.Exec(`INSERT INTO meta(key,value) VALUES('codex_settings','{"retries":3,"revision":4}')`); err != nil {
		t.Fatal(err)
	}
	out, err := s.CodexSettings()
	if err != nil || !out.AutoCompact || out.CompactPercent != 90 || out.Retries != 3 || out.Revision != 4 {
		t.Fatal(out, err)
	}
	disabled, percent := false, 80
	out, err = s.SaveCodexSettings(SaveCodexSettings{AutoCompact: &disabled, CompactPercent: &percent, ExpectedRevision: out.Revision})
	if err != nil || out.AutoCompact || out.CompactPercent != 80 || out.Retries != 3 {
		t.Fatal(out, err)
	}
	for _, bad := range []int{0, 100, -1} {
		if _, err = s.SaveCodexSettings(SaveCodexSettings{CompactPercent: &bad, ExpectedRevision: out.Revision}); !errors.Is(err, ErrInvalid) {
			t.Fatal(bad, err)
		}
	}
	retries := 7
	out, err = s.SaveCodexSettings(SaveCodexSettings{Retries: &retries, ExpectedRevision: out.Revision})
	if err != nil || out.AutoCompact || out.CompactPercent != 80 {
		t.Fatal(out, err)
	}
	got, err := s.CodexSettings()
	if err != nil || got != out {
		t.Fatal(got, out, err)
	}
}

func TestCodexCatalogConsentDefaultsAndPartialUpdates(t *testing.T) {
	s, err := Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	out, err := s.CodexSettings()
	if err != nil || out.OverrideModelCatalog {
		t.Fatal("unsafe default", out, err)
	}
	if _, err = s.db.Exec(`INSERT INTO meta(key,value) VALUES('codex_settings','{"retries":3,"revision":4}')`); err != nil {
		t.Fatal(err)
	}
	out, err = s.CodexSettings()
	if err != nil || out.OverrideModelCatalog {
		t.Fatal("legacy state implicitly opted in", out, err)
	}
	on := true
	out, err = s.SaveCodexSettings(SaveCodexSettings{OverrideModelCatalog: &on, ExpectedRevision: 4})
	if err != nil || !out.OverrideModelCatalog {
		t.Fatal(out, err)
	}
	retries := 2
	out, err = s.SaveCodexSettings(SaveCodexSettings{Retries: &retries, ExpectedRevision: out.Revision})
	if err != nil || !out.OverrideModelCatalog {
		t.Fatal("partial update lost consent", out, err)
	}
	off := false
	out, err = s.SaveCodexSettings(SaveCodexSettings{OverrideModelCatalog: &off, ExpectedRevision: out.Revision})
	if err != nil || out.OverrideModelCatalog {
		t.Fatal(out, err)
	}
}
