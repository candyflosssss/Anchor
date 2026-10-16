package logbook

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestAgentFailureAttributeAllowlistIsEventScoped(t *testing.T) {
	s := openTest(t)
	l := slog.New(NewHandler(slog.NewTextHandler(io.Discard, nil), s)).With("action", "sync", "private", "DO_NOT_STORE")
	l.Warn("agent operation failed", "stage", "verify_written", "reason", "managed_field_mismatch", "field", "dsh.default_model", "adapter_id", "dsh-desktop", "binding_id", "desktop-fixture", "error", "DO_NOT_STORE", "path", "DO_NOT_STORE", "oversized", strings.Repeat("x", 129))
	l.Info("core ready", "instance_id", "fixture", "stage", "DO_NOT_STORE", "reason", "DO_NOT_STORE")
	flushTest(t, s)
	page, err := s.List(t.Context(), Query{Kind: "system"})
	if err != nil || len(page.Events) != 2 {
		t.Fatalf("events: %+v %v", page, err)
	}
	for _, event := range page.Events {
		if event.Event == "core.started" {
			if len(event.Fields) != 1 || event.Fields["instance_id"] != "fixture" {
				t.Fatalf("diagnostic attributes escaped their event: %+v", event)
			}
			continue
		}
		if event.Event != "agent.operation_failed" || len(event.Fields) != 6 || event.Fields["stage"] != "verify_written" || event.Fields["reason"] != "managed_field_mismatch" || event.Fields["field"] != "dsh.default_model" || event.Fields["action"] != "sync" {
			t.Fatalf("unsafe or missing fields: %+v", event)
		}
	}
}
