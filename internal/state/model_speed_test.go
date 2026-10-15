package state

import (
	"errors"
	"github.com/candyflosssss/Anchor/internal/modelconfig"
	"strings"
	"testing"
)

func TestSpeedVariantAtomicCreation(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"p", "other"} {
		if _, err := s.PutProvider(provider(id), 0, nil); err != nil {
			t.Fatal(err)
		}
	}
	base := Model{ID: "gpt-6.1-sol", Tools: true, Streaming: true, Candidates: []Candidate{{ProviderID: "p", Model: "gpt-6.1-sol"}, {ProviderID: "other", Model: "other-upstream"}}, Settings: &modelconfig.ModelSettings{}}
	// A stale provider revision must not leave either the base or any sibling behind.
	if _, err := s.PutProviderModelVariants(base, 0, "p", 2, []string{"fast", "ultrafast"}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	for _, id := range []string{base.ID, base.ID + "(fast)", base.ID + "(ultrafast)"} {
		if _, err := s.Model(id); !errors.Is(err, ErrNotFound) {
			t.Fatal("stale partial write", id, err)
		}
	}
	saved, err := s.PutProviderModelVariants(base, 0, "p", 1, []string{"fast", "ultrafast"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 1 || saved.ServiceTier != "" {
		t.Fatal(saved)
	}
	for _, speed := range []string{"fast", "ultrafast"} {
		id, tier, _ := SpeedVariantID(base.ID, speed)
		got, err := s.Model(id)
		if err != nil || got.ServiceTier != tier || got.Revision != 1 || len(got.Candidates) != 1 || got.Candidates[0].ProviderID != "p" || got.Candidates[0].Model != base.ID || got.Settings.Reference != nil || !got.Tools || !got.Streaming {
			t.Fatal(got, err)
		}
	}
	// Collision rolls back even an otherwise valid update of the base.
	saved.Tools = false
	if _, err := s.PutProviderModelVariants(saved, 1, "p", 1, []string{"fast"}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	got, _ := s.Model(base.ID)
	if got.Revision != 1 || !got.Tools {
		t.Fatal("partial write", got)
	}
	// A later sibling conflict also rolls back the first sibling.
	existing := Model{ID: "second(ultrafast)", ServiceTier: "ultrafast", Candidates: base.Candidates}
	if _, err := s.PutModel(existing, 0); err != nil {
		t.Fatal(err)
	}
	base.ID = "second"
	if _, err := s.PutProviderModelVariants(base, 0, "p", 1, []string{"fast", "ultrafast"}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	for _, id := range []string{"second", "second(fast)"} {
		if _, err := s.Model(id); !errors.Is(err, ErrNotFound) {
			t.Fatal("partial sibling", id, err)
		}
	}
}

func TestSpeedVariantValidation(t *testing.T) {
	for _, id := range []string{"gpt-6.1-sol(fast)", "中文(ultrafast)", strings.Repeat("鱼", 122) + "(fast)"} {
		if !ValidModelID(id) {
			t.Error(id)
		}
		if ValidID(id) {
			t.Error("other identifiers widened", id)
		}
	}
	for _, id := range []string{"model(other)", "model(fast)(ultrafast)", "model(fast)/path", strings.Repeat("鱼", 123) + "(fast)"} {
		if ValidModelID(id) {
			t.Error(id)
		}
	}
	m := Model{ID: "base", Candidates: []Candidate{{ProviderID: "p", Model: "up"}}}
	for _, speeds := range [][]string{{"fast", "fast"}, {"other"}, {"fast", "ultrafast", "fast"}} {
		if _, err := speedModels(m, "p", speeds); !errors.Is(err, ErrInvalid) {
			t.Fatal(speeds, err)
		}
	}
	m.ServiceTier = "auto"
	if m.Validate() == nil {
		t.Fatal("unknown fixed tier accepted")
	}
	m.ServiceTier = "priority"
	if _, err := speedModels(m, "p", []string{"ultrafast"}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
