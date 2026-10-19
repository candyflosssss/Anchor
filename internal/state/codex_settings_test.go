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
