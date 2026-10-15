package core

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestAgentErrorCategoriesNeverLeakPrivateCauses(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{state.ErrConflict, "conflict"},
		{managed.ErrChanged, "agent_config_conflict"},
		{managed.ErrUnsafe, "agent_config_unsafe"},
		{managed.ErrModelUnavailable, "agent_model_unavailable"},
		{managed.ErrCatalogInvalid, "agent_catalog_invalid"},
		{errors.Join(managed.ErrCatalogInvalid, managed.ErrUnsafe), "agent_catalog_invalid"},
		{errors.Join(managed.ErrModelUnavailable, state.ErrNotFound), "agent_model_unavailable"},
		{errors.New("unclassified parser failure"), "agent_operation_failed"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			w := httptest.NewRecorder()
			agentError(w, fmt.Errorf("private-fixture /private/config: %w", tc.err))
			body := w.Body.String()
			if w.Code != 409 || !strings.Contains(body, `"code":"`+tc.code+`"`) {
				t.Fatalf("unexpected response %d: %s", w.Code, body)
			}
			if strings.Contains(body, "private-fixture") || strings.Contains(body, "/private/config") || strings.Contains(body, "unclassified parser") {
				t.Fatal("private error cause crossed API boundary")
			}
		})
	}
}
