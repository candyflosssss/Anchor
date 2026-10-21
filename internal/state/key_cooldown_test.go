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
	in := SaveKeyCooldownSettings{Enabled: &enabled, FailureThreshold: &threshold, CooldownSeconds: &seconds, RateLimitSeconds: &rate, ExpectedRevision: 1}
	out, err = s.SaveKeyCooldownSettings(in)
	if err != nil || out.Enabled || out.FailureThreshold != 7 || out.CooldownSeconds != 90 || out.RateLimitSeconds != 120 || out.ErrorWindowSeconds != 300 || out.Revision != 2 {
		t.Fatal(out, err)
	}
	if _, err = s.SaveKeyCooldownSettings(in); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	in.ExpectedRevision = 2
	for _, bad := range []SaveKeyCooldownSettings{{}, {Enabled: nil, FailureThreshold: &threshold, CooldownSeconds: &seconds, RateLimitSeconds: &rate, ExpectedRevision: 2}, {Enabled: &enabled, FailureThreshold: nil, CooldownSeconds: &seconds, RateLimitSeconds: &rate, ExpectedRevision: 2}, {Enabled: &enabled, FailureThreshold: &threshold, CooldownSeconds: nil, RateLimitSeconds: &rate, ExpectedRevision: 2}, {Enabled: &enabled, FailureThreshold: &threshold, CooldownSeconds: &seconds, RateLimitSeconds: nil, ExpectedRevision: 2}} {
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
	window := 0
	in.ErrorWindowSeconds = &window
	for _, n := range []int{0, 3601} {
		window = n
		if _, err = s.SaveKeyCooldownSettings(in); !errors.Is(err, ErrInvalid) {
			t.Fatal(n, err)
		}
	}
	in.ErrorWindowSeconds = nil
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

func TestKeyCooldownSettingsMigrationAndDeprecatedWeight(t *testing.T) {
	s, err := Open(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec(`INSERT INTO meta(key,value) VALUES('key_cooldown_settings','{"enabled":false,"failure_threshold":4,"cooldown_seconds":40,"rate_limit_seconds":50,"revision":7}')`)
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.KeyCooldownSettings()
	if err != nil || old.WeightPercent != 10 || old.Enabled || old.Revision != 7 || old.ErrorWindowSeconds != 300 || old.FailureThreshold != 100 {
		t.Fatal(old, err)
	}
	weight := 25
	in := SaveKeyCooldownSettings{WeightPercent: &weight, Enabled: &old.Enabled, FailureThreshold: &old.FailureThreshold, CooldownSeconds: &old.CooldownSeconds, RateLimitSeconds: &old.RateLimitSeconds, ExpectedRevision: 7}
	out, err := s.SaveKeyCooldownSettings(in)
	if err != nil || out.WeightPercent != 25 || out.FailureThreshold != 100 {
		t.Fatal(out, err)
	}
	in.ExpectedRevision = 8
	for _, n := range []int{0, 101} {
		weight = n
		if _, err = s.SaveKeyCooldownSettings(in); err != ErrInvalid {
			t.Fatal(n, err)
		}
	}
	// Old clients omit the new field and preserve its stored value.
	in.WeightPercent = nil
	out, err = s.SaveKeyCooldownSettings(in)
	if err != nil || out.WeightPercent != 25 || out.Revision != 9 {
		t.Fatal(out, err)
	}
}
