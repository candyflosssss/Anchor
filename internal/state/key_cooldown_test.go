package state

import (
	"errors"
	"testing"
)

func TestKeyCooldownDefaultPersistenceAndValidation(t *testing.T) {
	s, err := Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	out, err := s.KeyCooldownSettings()
	if err != nil || out != DefaultKeyCooldownSettings() {
		t.Fatal(out, err)
	}
	enabled, threshold, seconds, rate := false, 7, 90, 120
	in := SaveKeyCooldownSettings{&enabled, &threshold, &seconds, &rate, 1}
	out, err = s.SaveKeyCooldownSettings(in)
	if err != nil || out.Enabled || out.FailureThreshold != 7 || out.CooldownSeconds != 90 || out.RateLimitSeconds != 120 || out.Revision != 2 {
		t.Fatal(out, err)
	}
	if _, err = s.SaveKeyCooldownSettings(in); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	in.ExpectedRevision = 2
	for _, bad := range []SaveKeyCooldownSettings{{}, {nil, &threshold, &seconds, &rate, 2}, {&enabled, nil, &seconds, &rate, 2}, {&enabled, &threshold, nil, &rate, 2}, {&enabled, &threshold, &seconds, nil, 2}} {
		if _, err = s.SaveKeyCooldownSettings(bad); !errors.Is(err, ErrInvalid) {
			t.Fatal(bad, err)
		}
	}
	for _, n := range []int{0, 101} {
		threshold = n
		if _, err = s.SaveKeyCooldownSettings(in); !errors.Is(err, ErrInvalid) {
			t.Fatal(n, err)
		}
	}
	threshold = 7
	for _, target := range []*int{&seconds, &rate} {
		for _, n := range []int{0, 3601} {
			*target = n
			if _, err = s.SaveKeyCooldownSettings(in); !errors.Is(err, ErrInvalid) {
				t.Fatal(n, err)
			}
		}
		*target = 30
	}
	for _, n := range []int64{0, 9007199254740991} {
		in.ExpectedRevision = n
		if _, err = s.SaveKeyCooldownSettings(in); !errors.Is(err, ErrInvalid) {
			t.Fatal(n, err)
		}
	}
	reopened, err := Open(s.Dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.KeyCooldownSettings()
	if err != nil || got != out {
		t.Fatal(got, out, err)
	}
}
