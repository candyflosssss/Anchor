package core

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/state"
)

func logAttribution(r *http.Request, t *requestTrace) {
	credentials := []string{strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), r.Header.Get("x-api-key")}
	t.row.Project = logIdentity(r.Header.Values("X-Anchor-Project"), credentials...)
	t.row.Session = logIdentity(r.Header.Values("X-Anchor-Session"), credentials...)
	if a, ok := r.Context().Value(accessKey{}).(state.Access); ok {
		t.row.AccessKey = a.ID
		// A user may paste a credential into a label. Never persist it as identity.
		t.row.AccessLabel = a.Label
		for _, credential := range credentials {
			if credential != "" && strings.Contains(t.row.AccessLabel, credential) {
				t.row.AccessLabel = ""
			}
		}
	}
	b, ok := r.Context().Value(bindingKey{}).(state.Binding)
	if !ok {
		return
	}
	t.row.Agent, t.row.Binding = b.AdapterID, b.ID
	if b.AdapterID != "codex" {
		return
	}
	native := codexLogSession(r.Header, credentials...)
	if t.row.Session == "" {
		t.row.Session = native
	}
	if t.row.Project == "" && native != "" {
		t.row.Project = managed.CodexLogProject(r.Context(), b.ConfigRoot, native)
	}
}

// Current Codex sends structured turn metadata; older clients send session_id.
// Prefer thread_id (the actual child thread), never turn/window/cache IDs. Native
// metadata is still caller-declared and confers no authentication or routing power.
func codexLogSession(header http.Header, credentials ...string) string {
	if values := header.Values("x-codex-turn-metadata"); len(values) == 1 && len(values[0]) <= 8192 {
		var meta struct {
			Thread  string `json:"thread_id"`
			Session string `json:"session_id"`
		}
		if json.Unmarshal([]byte(values[0]), &meta) == nil {
			for _, value := range []string{meta.Thread, meta.Session} {
				if validCodexThread(value) && logIdentity([]string{value}, credentials...) != "" {
					return strings.ToLower(value)
				}
			}
		}
	}
	if value := logIdentity(header.Values("session_id"), credentials...); validCodexThread(value) {
		return strings.ToLower(value)
	}
	return ""
}

func validCodexThread(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, c := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}
