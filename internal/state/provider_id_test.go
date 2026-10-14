package state

import (
	"strings"
	"testing"
)

func TestProviderIDValidation(t *testing.T) {
	for _, id := range []string{"primary", "摸鱼AI", "中文供应商-备用_2.0", "𠮷野家", "Équipe", "供应商１２", strings.Repeat("鱼", 128)} {
		t.Run(id, func(t *testing.T) {
			if !ValidProviderID(id) {
				t.Fatal("valid provider ID rejected")
			}
			p := provider(id)
			if err := p.Validate(); err != nil {
				t.Fatal(err)
			}
			m := Model{ID: "public", Candidates: []Candidate{{ProviderID: id, Model: "actual"}}}
			if err := m.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, id := range []string{"", strings.Repeat("鱼", 129), strings.Repeat("a", 129), "-中文", ".中文", "_中文", "摸 鱼", "摸鱼\n", "摸鱼\r", "摸鱼\t", "摸鱼\x00", "摸鱼/AI", `摸鱼\AI`, "摸鱼:AI", "摸鱼?AI", "摸鱼#AI", "摸鱼%2FAI", "摸鱼😀", "摸鱼\u200bAI", "摸鱼\u202eAI", "摸鱼\u0301", string([]byte{0xff})} {
		t.Run("invalid/"+id, func(t *testing.T) {
			if ValidProviderID(id) {
				t.Fatal("invalid provider ID accepted")
			}
			if err := provider(id).Validate(); err == nil {
				t.Fatal("provider accepted invalid ID")
			}
			m := Model{ID: "public", Candidates: []Candidate{{ProviderID: id, Model: "actual"}}}
			if err := m.Validate(); err == nil {
				t.Fatal("candidate accepted invalid provider ID")
			}
		})
	}
	if !ValidID("摸鱼AI") {
		t.Fatal("user-facing identifiers must support Chinese")
	}
}

func TestChineseProviderPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	secret := "isolated-test-secret"
	p := provider("摸鱼AI")
	if _, err = s.PutProvider(p, 0, &secret); err != nil {
		t.Fatal(err)
	}
	m := Model{ID: "public", Candidates: []Candidate{{ProviderID: p.ID, Model: "actual"}}}
	if _, err = s.PutModel(m, 0); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Provider(p.ID)
	if err != nil || got.ID != p.ID || got.Secret != secret {
		t.Fatal("provider did not persist", err)
	}
	models, err := reopened.Models()
	if err != nil || len(models) != 1 || models[0].Candidates[0].ProviderID != p.ID {
		t.Fatal("candidate did not persist", err)
	}
}
