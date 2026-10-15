package managed

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func assertDSHValue(t *testing.T, raw []byte, path []string, want any) {
	t.Helper()
	values, err := (StructuredEditor{DSH: true}).Values(raw, [][]string{path})
	if err != nil || !equal(values[0], Value{Exists: want != nil, Data: normalize(want)}) {
		t.Fatalf("path=%v values=%v want=%v err=%v", path, values, want, err)
	}
}

func TestDSHCapabilitiesReconcileAndRecoverBothProfiles(t *testing.T) {
	_, base, g, engine := target(t, "")
	g.Token = "synthetic-binding-token"
	original := []byte("# keep comment\n- id: llm-pi-ai\n  config:\n    providers: {user: {keep: true}}\n- id: agent-default-model\n  config: {provider: original, model: old, reasoningEffort: medium, keep: true}\n- id: unrelated\n  config: {keep: true}\n")
	bindings := []state.Binding{}
	for _, adapter := range []string{"dsh", "dsh-desktop"} {
		b := base
		b.ID, b.AdapterID = adapter, adapter
		a := AdapterFor(adapter).(StructuredAgent)
		path := a.path(b)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
		p, err := a.Plan(b, g)
		if err != nil {
			t.Fatal(err)
		}
		// Simulate the old adapter's journal (provider+model only). The new
		// projection must safely extend ownership without losing this baseline.
		p.Edits = p.Edits[:len(p.Edits)-1]
		if _, err = engine.Apply(p); err != nil {
			t.Fatal(err)
		}
		bindings = append(bindings, b)
	}
	g.Models = []state.Model{{ID: "public", DisplayName: "Public name", Settings: &state.ModelSettings{Configuration: dshSettingsFixture()}}, {ID: "other", Settings: &state.ModelSettings{Configuration: state.ModelConfiguration{SupportedReasoning: []modeldev.ReasoningOption{{Type: "effort", Values: []*string{settingsPtr("low")}}}, DefaultReasoning: &state.ReasoningDefault{Type: "effort", Value: settingsPtr("low")}}}}}
	for _, b := range bindings {
		a := AdapterFor(b.AdapterID).(StructuredAgent)
		other := bindings[0]
		if other.ID == b.ID {
			other = bindings[1]
		}
		otherPath := AdapterFor(other.AdapterID).(StructuredAgent).path(other)
		untouched := readSyncFile(t, otherPath)
		if r, err := engine.Reconcile(b, g); err != nil || r.Status != "applied" {
			t.Fatal(r, err)
		}
		if !bytes.Equal(untouched, readSyncFile(t, otherPath)) {
			t.Fatal("cross-profile write")
		}
		raw := readSyncFile(t, a.path(b))
		entry, _, _ := dshModel(g.Models[0])
		otherEntry, _, _ := dshModel(g.Models[1])
		assertDSHValue(t, raw, []string{"llm-pi-ai", "config", "providers", "anchor-" + b.ID, "models"}, []any{entry, otherEntry})
		assertDSHValue(t, raw, []string{"agent-default-model", "config", "reasoningEffort"}, "high")
		if _, err := a.Verify(b, g); err != nil {
			t.Fatal(err)
		}
		journalBefore := readSyncFile(t, engine.journalPath(b.ID))
		if _, err := engine.Reconcile(b, g); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw, readSyncFile(t, a.path(b))) || !bytes.Equal(journalBefore, readSyncFile(t, engine.journalPath(b.ID))) {
			t.Fatal("no-op churn")
		}
		// Switching the binding default selects only the new default's effort.
		b.Model = "other"
		if _, err := engine.Reconcile(b, g); err != nil {
			t.Fatal(err)
		}
		assertDSHValue(t, readSyncFile(t, a.path(b)), []string{"agent-default-model", "config", "reasoningEffort"}, "low")
		b.Model = "public"
		cleared := g
		cleared.Models = []state.Model{{ID: "public"}, {ID: "other"}}
		if _, err := engine.Reconcile(b, cleared); err != nil {
			t.Fatal(err)
		}
		raw = readSyncFile(t, a.path(b))
		assertDSHValue(t, raw, []string{"agent-default-model", "config", "reasoningEffort"}, nil)
		assertDSHValue(t, raw, []string{"llm-pi-ai", "config", "providers", "anchor-" + b.ID, "models"}, []any{map[string]any{"id": "public", "name": "public"}, map[string]any{"id": "other", "name": "other"}})
		// Invalid metadata fails before any write or journal update.
		bad := g
		bad.Models = []state.Model{{ID: "public", Settings: &state.ModelSettings{Configuration: state.ModelConfiguration{Limits: modeldev.CatalogLimits{Context: settingsPtr(int64(-1))}}}}}
		journalBefore = readSyncFile(t, engine.journalPath(b.ID))
		if _, err := engine.Reconcile(b, bad); err == nil {
			t.Fatal("invalid metadata accepted")
		}
		if !bytes.Equal(raw, readSyncFile(t, a.path(b))) || !bytes.Equal(journalBefore, readSyncFile(t, engine.journalPath(b.ID))) {
			t.Fatal("invalid metadata wrote config")
		}
		if r, err := engine.Recover(b.ID); err != nil || r.Status != "restored" {
			t.Fatal(r, err)
		}
		restored := readSyncFile(t, a.path(b))
		assertDSHValue(t, restored, []string{"agent-default-model", "config", "reasoningEffort"}, "medium")
		assertDSHValue(t, restored, []string{"agent-default-model", "config", "provider"}, "original")
		if !bytes.Contains(restored, []byte("# keep comment")) || bytes.Contains(restored, []byte("anchor-")) {
			t.Fatal("baseline not restored")
		}
	}
}

func TestDSHReasoningExternalEditIsNotOverwritten(t *testing.T) {
	for _, adapter := range []string{"dsh", "dsh-desktop"} {
		t.Run(adapter, func(t *testing.T) {
			_, b, g, engine := target(t, "")
			b.AdapterID = adapter
			g.Token = "synthetic"
			g.Models = []state.Model{{ID: "public", Settings: &state.ModelSettings{Configuration: dshSettingsFixture()}}}
			a := AdapterFor(adapter).(StructuredAgent)
			if err := os.MkdirAll(filepath.Dir(a.path(b)), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(a.path(b), []byte("- id: agent-default-model\n  config: {provider: old, model: old}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			p, err := a.Plan(b, g)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Apply(p); err != nil {
				t.Fatal(err)
			}
			raw := readSyncFile(t, a.path(b))
			changed, err := (StructuredEditor{DSH: true}).Patch(raw, []Edit{{Path: []string{"agent-default-model", "config", "reasoningEffort"}, Before: Value{Exists: true, Data: "high"}, After: Value{Exists: true, Data: "low"}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(a.path(b), changed, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Reconcile(b, g); !errors.Is(err, ErrChanged) {
				t.Fatal(err)
			}
			if !bytes.Equal(changed, readSyncFile(t, a.path(b))) {
				t.Fatal("external effort overwritten")
			}
			if r, _ := engine.Recover(b.ID); r.Status == "restored" {
				t.Fatal("external edit recovery was not conditional")
			}
		})
	}
}
