package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/state"
)

func agentLogBinding(t *testing.T, f *fixture) state.Binding {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.store.AddBinding(state.Binding{ID: "desktop-fixture", AdapterID: "dsh-desktop", AdapterVersion: "1", ConfigRoot: t.TempDir(), Model: "public", Executable: exe, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(b.ConfigRoot, "profiles", "desktop"), 0700); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAgentFailuresPersistSafeDiagnostics(t *testing.T) {
	f := setup(t)
	b := agentLogBinding(t, f)
	for _, tc := range []struct {
		err    error
		reason string
	}{
		{managed.ErrChanged, "configuration_changed"},
		{managed.ErrUnsafe, "unsafe_configuration"},
		{managed.ErrCatalogInvalid, "catalog_invalid"},
		{errors.New("private-fixture"), "operation_failed"},
	} {
		w := httptest.NewRecorder()
		f.core.agentError(w, "sync", b.ID, fmt.Errorf("private-fixture /private/config: %w", tc.err))
		if w.Code != 409 || bytes.Contains(w.Body.Bytes(), []byte("private-fixture")) {
			t.Fatalf("unsafe API response: %d %s", w.Code, w.Body)
		}
		if err := f.core.logs.Flush(t.Context()); err != nil {
			t.Fatal(err)
		}
		page, err := f.core.logs.List(t.Context(), logbook.Query{Kind: "system", Limit: 1})
		if err != nil || len(page.Events) != 1 {
			t.Fatalf("missing event: %+v %v", page, err)
		}
		e := page.Events[0]
		if e.Event != "agent.operation_failed" || e.Level != "warn" || e.Fields["reason"] != tc.reason || e.Fields["stage"] != "operation" || e.Fields["action"] != "sync" || e.Fields["adapter_id"] != "dsh-desktop" || e.Fields["binding_id"] != b.ID {
			t.Fatalf("wrong diagnostic: %+v", e)
		}
	}
	c := logClient(t, f)
	var exported bytes.Buffer
	if err := c.ExportLogs(t.Context(), logbook.Query{Kind: "system"}, &exported); err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-fixture", "/private/config", b.ConfigRoot} {
		if bytes.Contains(exported.Bytes(), []byte(private)) || bytes.Contains(f.logs.Bytes(), []byte(private)) {
			t.Fatalf("private value leaked: %s", private)
		}
	}
}

func TestAgentRouteFailureLoggedButSuccessIsNot(t *testing.T) {
	f := setup(t)
	b := agentLogBinding(t, f)
	c := logClient(t, f)
	if _, err := c.AgentQuery(t.Context(), b.ID, "inspect"); err != nil {
		t.Fatal(err)
	}
	if err := f.core.logs.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err := c.Logs(t.Context(), logbook.Query{Kind: "system"})
	if err != nil || len(page.Events) != 0 {
		t.Fatalf("success recorded as failure: %+v %v", page, err)
	}
	// Stale revision fails before any read/write of the fixture configuration.
	if _, err := c.AgentAction(t.Context(), b.ID, "apply", b.Revision+1, "reviewed-fixture"); err == nil {
		t.Fatal("stale revision accepted")
	}
	if err := f.core.logs.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err = c.Logs(t.Context(), logbook.Query{Kind: "system"})
	if err != nil || len(page.Events) != 1 || page.Events[0].Fields["reason"] != "revision_conflict" || page.Events[0].Fields["action"] != "apply" {
		t.Fatalf("route failure not logged: %+v %v", page, err)
	}
	f.core.AgentEnvironment = func() (managed.Environment, error) {
		return managed.Environment{}, errors.New("private-fixture /private/config")
	}
	if _, err := c.AgentWorkspace(t.Context()); err == nil {
		t.Fatal("environment failure accepted")
	}
	if err := f.core.logs.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err = c.Logs(t.Context(), logbook.Query{Kind: "system"})
	if err != nil || len(page.Events) != 2 || page.Events[0].Fields["action"] != "workspace" {
		t.Fatalf("workspace failure not logged: %+v %v", page, err)
	}
	raw, _ := json.Marshal(page)
	if bytes.Contains(raw, []byte("private-fixture")) || bytes.Contains(f.logs.Bytes(), []byte("private-fixture")) {
		t.Fatal("environment error leaked")
	}
}
