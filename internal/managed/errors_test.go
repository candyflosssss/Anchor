package managed

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestManagedCatalogFailurePreservesCategoryAndCause(t *testing.T) {
	_, _, err := buildCatalogTemplates([]state.Model{{ID: "broken", Codex: &state.CodexMetadata{}}}, "", nil)
	if !errors.Is(err, ErrCatalogInvalid) {
		t.Fatal("catalog failure lost safe category", err)
	}
	_, _, err = buildCatalogTemplates([]state.Model{{ID: "broken"}}, "", map[string]json.RawMessage{"broken": []byte(`null`)})
	if !errors.Is(err, ErrCatalogInvalid) || !errors.Is(err, ErrUnsafe) {
		t.Fatal("catalog wrapper lost original unsafe cause", err)
	}
}

func TestUnavailableManagedDefaultHasDistinctCategory(t *testing.T) {
	m, _, _ := workspaceManager(t)
	_, err := m.gateway(state.Binding{Model: "missing", ModelScope: "all-mapped"})
	if !errors.Is(err, ErrModelUnavailable) || !errors.Is(err, state.ErrNotFound) {
		t.Fatal("missing default lost category or cause", err)
	}
	p, err := m.Store.Provider("p")
	if err != nil {
		t.Fatal(err)
	}
	p.Enabled = false
	if _, err := m.Store.PutProvider(p, p.Revision, nil); err != nil {
		t.Fatal(err)
	}
	_, err = m.gateway(state.Binding{Model: "public", ModelScope: "all-mapped"})
	if !errors.Is(err, ErrModelUnavailable) {
		t.Fatal("disabled provider not distinguished from config conflict", err)
	}
}
