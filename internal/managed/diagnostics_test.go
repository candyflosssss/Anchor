package managed

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestDiagnosticSafeCategoriesAndWrapping(t *testing.T) {
	for _, tc := range []struct {
		err    error
		reason string
	}{
		{ErrChanged, "configuration_changed"},
		{ErrUnsafe, "unsafe_configuration"},
		{ErrModelUnavailable, "model_unavailable"},
		{ErrCatalogInvalid, "catalog_invalid"},
		{state.ErrConflict, "revision_conflict"},
		{state.ErrNotFound, "not_found"},
		{os.ErrPermission, "permission_denied"},
		{&os.PathError{Op: "private-fixture", Path: "/private/config", Err: errors.New("private-fixture")}, "filesystem_error"},
		{errors.New("private-fixture"), "operation_failed"},
	} {
		err := fmt.Errorf("private-fixture /private/config: %w", tc.err)
		d := Diagnose(err)
		if d.Stage != "operation" || d.Reason != tc.reason || d.Field != "" {
			t.Fatalf("unexpected diagnostic: %+v", d)
		}
		annotated := withDiagnostic(err, "verify_written", "", "managed_field")
		if !errors.Is(annotated, tc.err) || Diagnose(annotated).Reason != tc.reason {
			t.Fatal("annotation changed cause/category")
		}
		raw, _ := json.Marshal(Diagnose(annotated))
		if bytes.Contains(raw, []byte("private-fixture")) || bytes.Contains(raw, []byte("/private/config")) {
			t.Fatal("private cause leaked into diagnostic")
		}
	}
	if withDiagnostic(nil, "verify_written", "", "") != nil {
		t.Fatal("successful operation became an error")
	}
	for _, path := range [][]string{{"llm-pi-ai", "config", "providers", "private-fixture"}, {"private-fixture"}, {"env", "private-fixture"}} {
		if strings.Contains(diagnosticField(path), "private-fixture") {
			t.Fatal("untrusted field path entered diagnostic")
		}
	}
}

func TestApplyFailureDiagnostics(t *testing.T) {
	t.Run("unfinished_journal_before_first_write", func(t *testing.T) {
		_, b, g, engine := target(t, "model = \"old\"\n")
		p, err := (Codex{}).Plan(b, g)
		if err != nil {
			t.Fatal(err)
		}
		if err := engine.prepare(); err != nil {
			t.Fatal(err)
		}
		if err := engine.save(journal{Schema: 1, BindingID: b.ID, AdapterVersion: "1", OperationID: "prepared", Path: p.Path, Original: p.original, Existed: true, Edits: p.Edits, Status: "prepared"}); err != nil {
			t.Fatal(err)
		}
		_, err = engine.Apply(p)
		if d := Diagnose(err); !errors.Is(err, ErrChanged) || d.Stage != "apply_precondition" || d.Reason != "unfinished_journal" {
			t.Fatalf("%+v %v", d, err)
		}
		raw, err := os.ReadFile(p.Path)
		if err != nil || !bytes.Equal(raw, p.original) {
			t.Fatal("precondition failure modified configuration", err)
		}
	})
	t.Run("changed_since_plan", func(t *testing.T) {
		_, b, g, engine := target(t, "model = \"old\"\n")
		p, err := (Codex{}).Plan(b, g)
		if err != nil {
			t.Fatal(err)
		}
		changed := []byte("model = \"private-fixture\"\n")
		if err := os.WriteFile(p.Path, changed, 0600); err != nil {
			t.Fatal(err)
		}
		_, err = engine.Apply(p)
		if d := Diagnose(err); !errors.Is(err, ErrChanged) || d.Stage != "read_configuration" || d.Reason != "configuration_changed_since_plan" {
			t.Fatalf("%+v %v", d, err)
		}
		err = atomicWrite(filepath.Dir(p.Path), p.Path, p.original, p.Fingerprint, true)
		if d := Diagnose(err); !errors.Is(err, ErrChanged) || d.Stage != "write_configuration" || d.Reason != "configuration_changed_before_write" {
			t.Fatalf("%+v %v", d, err)
		}
		raw, err := os.ReadFile(p.Path)
		if err != nil || !bytes.Equal(raw, changed) {
			t.Fatal("external edits overwritten", err)
		}
		if _, err := os.Stat(engine.journalPath(b.ID)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("stale plan created a journal", err)
		}
	})
}

// Simulate a faulty write in an isolated fixture, not a change by the user.
type mismatchingEditor struct{ TOMLEditor }

func (e mismatchingEditor) Patch(raw []byte, edits []Edit) ([]byte, error) {
	next, err := e.TOMLEditor.Patch(raw, edits)
	return bytes.Replace(next, []byte(`model = "public"`), []byte(`model = "private-fixture"`), 1), err
}

func TestManagerDistinguishesPostWriteVerification(t *testing.T) {
	st, b, _, _ := target(t, "model = \"old\"\n")
	if _, err := st.PutProvider(state.Provider{ID: "p", BaseURL: "http://127.0.0.1:1/v1", Protocol: "responses", Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutModel(state.Model{ID: b.Model, Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "fixture"}}}, 0); err != nil {
		t.Fatal(err)
	}
	var err error
	b, err = st.AddBinding(b)
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(st)
	m.ready = true
	m.Engine.Editor = mismatchingEditor{}
	result, err := m.Apply(b.ID, b.Revision, "")
	if d := Diagnose(err); !errors.Is(err, ErrChanged) || d.Stage != "verify_written" || d.Reason != "managed_field_mismatch" || result.Status != "applied" {
		t.Fatalf("post-write mismatch: %+v %+v %v", d, result, err)
	}
	_, err = m.Apply(b.ID, b.Revision, "")
	if d := Diagnose(err); !errors.Is(err, ErrChanged) || d.Stage != "apply_precondition" || d.Reason != "managed_field_mismatch" {
		t.Fatalf("existing mismatch: %+v %v", d, err)
	}
}
