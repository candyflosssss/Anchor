package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestShutdownAuthorizationAndLifecycle(t *testing.T) {
	f := setup(t)
	cfg, err := f.store.Settings()
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(token, identity, origin, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17861/control/v1/shutdown", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Anchor-Instance", identity)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		f.core.Control().ServeHTTP(w, req)
		return w
	}
	cases := []struct {
		token, identity, origin, body string
		want                          int
	}{
		{"", cfg.InstanceID, "", "{}", 401},
		{f.token, cfg.InstanceID, "", "{}", 401},
		{f.owner, "wrong", "", "{}", 409},
		{f.owner, cfg.InstanceID, "http://localhost", "{}", 403},
		{f.owner, cfg.InstanceID, "", `{"kill":true}`, 400},
		{f.owner, cfg.InstanceID, "", "null", 400},
		{f.owner, cfg.InstanceID, "", "", 400},
		{f.owner, cfg.InstanceID, "", "{}", 503}, // handler alone cannot stop a lifecycle
	}
	for _, tc := range cases {
		if w := invoke(tc.token, tc.identity, tc.origin, tc.body); w.Code != tc.want {
			t.Fatalf("shutdown status: got %d want %d", w.Code, tc.want)
		}
		select {
		case <-f.core.shutdown:
			t.Fatal("invalid request triggered stop")
		default:
		}
	}
	f.core.running.Store(true)
	if w := invoke(f.owner, cfg.InstanceID, "", "{}"); w.Code != 200 || !strings.Contains(w.Body.String(), `"accepted":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case <-f.core.shutdown:
	default:
		t.Fatal("shutdown was not requested")
	}
	if !f.core.stopping.Load() || f.core.restartHandoff.Load() {
		t.Fatal("ordinary stop must gate without selecting restart")
	}
	if w := invoke(f.owner, cfg.InstanceID, "", "{}"); w.Code != 503 {
		t.Fatal("replayed stop accepted")
	}
	// Do not allow new mutations or gateway work while draining; status stays readable.
	for _, tc := range []struct {
		method, path, token string
		want                int
		gateway             bool
	}{
		{"PUT", "/control/v1/providers/p", f.owner, 503, false},
		{"GET", "/control/v1/status", f.owner, 200, false},
		{"GET", "/v1/models", f.token, 503, true},
	} {
		req := httptest.NewRequest(tc.method, "http://127.0.0.1:17861"+tc.path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		handler := f.core.Control()
		if tc.gateway {
			handler = f.core.Gateway()
		}
		handler.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("draining %s got %d", tc.path, w.Code)
		}
		if tc.path == "/control/v1/status" && !strings.Contains(w.Body.String(), `"core":"stopping"`) {
			t.Fatal("wrong lifecycle status")
		}
	}
}

func TestStartingRejectsMutationsAndGateway(t *testing.T) {
	f := setup(t)
	f.core.running.Store(true)
	for _, tc := range []struct {
		method, path, token string
		want                int
		gateway             bool
	}{
		{"PUT", "/control/v1/providers/p", f.owner, 503, false},
		{"GET", "/control/v1/status", f.owner, 200, false},
		{"GET", "/v1/models", f.token, 503, true},
	} {
		req := httptest.NewRequest(tc.method, "http://127.0.0.1:17861"+tc.path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		handler := f.core.Control()
		if tc.gateway {
			handler = f.core.Gateway()
		}
		handler.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("starting %s got %d", tc.path, w.Code)
		}
		if tc.path == "/control/v1/status" && !strings.Contains(w.Body.String(), `"core":"starting"`) {
			t.Fatal("wrong startup status")
		}
	}
}

func TestAuthenticatedInstanceReportsProcessIdentity(t *testing.T) {
	f := setup(t)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:17861/control/v1/instance", nil)
	req.Header.Set("Authorization", "Bearer "+f.owner)
	w := httptest.NewRecorder()
	f.core.Control().ServeHTTP(w, req)
	var result struct {
		ProcessID    int      `json:"process_id"`
		Capabilities []string `json:"capabilities"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.ProcessID != os.Getpid() {
		t.Fatal("process identity must reflect the authenticated serving process")
	}
	if !slices.Contains(result.Capabilities, "restart_handoff") {
		t.Fatal("restart handoff capability not advertised")
	}
}

func TestRestartShutdownValidatesIntentBeforeMutation(t *testing.T) {
	f := setup(t)
	cfg, err := f.store.Settings()
	if err != nil {
		t.Fatal(err)
	}
	f.core.running.Store(true)
	invoke := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:17861/control/v1/shutdown", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+f.owner)
		req.Header.Set("X-Anchor-Instance", cfg.InstanceID)
		w := httptest.NewRecorder()
		f.core.Control().ServeHTTP(w, req)
		return w
	}
	for _, body := range []string{`{"for_restart":null}`, `{"for_restart":"true"}`, `{"for_restart":1}`, `{"for_restart":{}}`, `{"for_restart":true,"extra":true}`, `null`, `[]`, `{"for_restart":true} {}`} {
		if w := invoke(body); w.Code != 400 {
			t.Fatalf("malformed restart accepted: %s: %d", body, w.Code)
		}
		if f.core.restartHandoff.Load() || f.core.stopping.Load() {
			t.Fatal("invalid request changed shutdown intent")
		}
	}
	if w := invoke(`{"for_restart":true}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"bindings_preserved":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if !f.core.restartHandoff.Load() || !f.core.stopping.Load() {
		t.Fatal("restart not selected")
	}
	if w := invoke(`{}`); w.Code != 503 || !f.core.restartHandoff.Load() {
		t.Fatal("ordinary stop changed accepted restart intent")
	}
}
