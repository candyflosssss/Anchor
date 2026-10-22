package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestBindingGatewayIsolationAndCustomTools(t *testing.T) {
	f := setup(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-provider-secret" {
			t.Error("wrong upstream auth")
		}
		var body map[string]any
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		if body["model"] != "actual" {
			t.Error(body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"response","object":"response","status":"completed","output":[]}`))
	}))
	defer upstream.Close()
	f.add(t, "p", upstream.URL, "fixture-provider-secret")
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	if _, e := f.store.PutModel(state.Model{ID: "other", Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}, 0); e != nil {
		t.Fatal(e)
	}
	exe, _ := os.Executable()
	b, e := f.store.AddBinding(state.Binding{ID: "b", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: t.TempDir(), Model: "public", Executable: exe, Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	token, _ := f.store.BindingToken(b.ID)
	resp := request(t, "GET", f.gateway.URL+"/v1/models", token, "")
	body := text(t, resp)
	if resp.StatusCode != 200 || strings.Contains(body, "other") || !strings.Contains(body, "public") {
		t.Fatal(resp.StatusCode, body)
	}
	for _, tc := range []struct {
		model string
		code  int
	}{{"public", 200}, {"other", 403}} {
		r := request(t, "POST", f.gateway.URL+"/v1/responses", token, `{"model":"`+tc.model+`","tools":[{"type":"custom","name":"apply_patch","format":{"type":"text"}}],"input":[{"type":"custom_tool_call_output","call_id":"fixture","output":"done"}]}`)
		if r.StatusCode != tc.code {
			t.Fatal(r.StatusCode, text(t, r))
		}
	}
	if r := request(t, "GET", f.control.URL+"/control/v1/status", token, ""); r.StatusCode != 401 {
		t.Fatal("binding token managed core")
	}
	if _, e = f.store.EnableBinding(b.ID, 1, false); e != nil {
		t.Fatal(e)
	}
	if r := request(t, "GET", f.gateway.URL+"/v1/models", token, ""); r.StatusCode != 401 {
		t.Fatal("disabled binding authorized")
	}
}
func controlWrite(t *testing.T, f *fixture, path string, body any, identity bool) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	r, e := http.NewRequest("POST", f.control.URL+path, strings.NewReader(string(b)))
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Authorization", "Bearer "+f.owner)
	if identity {
		cfg, _ := f.store.Settings()
		r.Header.Set("X-Anchor-Instance", cfg.InstanceID)
	}
	r.Header.Set("Content-Type", "application/json")
	resp, e := http.DefaultClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}
func TestApprovedBindingPlanApplyRestoreAPI(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:8080/v1", "")
	f.model(t, state.Candidate{ProviderID: "p", Model: "actual"})
	root := t.TempDir()
	_ = os.Chmod(root, 0700)
	path := filepath.Join(root, "config.toml")
	original := `model = "original"` + "\nprivate = \"private-fixture\"\n"
	_ = os.WriteFile(path, []byte(original), 0600)
	exe, _ := os.Executable()
	b := state.Binding{ID: "codex-local", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: root, Model: "public", Executable: exe, Enabled: true}
	for _, tc := range []struct {
		approve, identity bool
		status            int
	}{{true, false, 409}, {false, true, 400}, {true, true, 200}} {
		r := controlWrite(t, f, "/control/v1/agents", map[string]any{"binding": b, "approved": tc.approve}, tc.identity)
		if r.StatusCode != tc.status {
			t.Fatal(r.StatusCode, text(t, r))
		}
	}
	unchanged, _ := os.ReadFile(path)
	if string(unchanged) != original {
		t.Fatal("add changed Agent config")
	}
	if _, e := f.core.Agents.Start(); e != nil {
		t.Fatal(e)
	}
	// Startup took over this approved binding. Restoring does not disable approval.
	r := controlWrite(t, f, "/control/v1/agents/codex-local/restore", map[string]any{"expected_revision": 1}, true)
	if r.StatusCode != 200 {
		t.Fatal(text(t, r))
	}
	plan := request(t, "GET", f.control.URL+"/control/v1/agents/codex-local/plan", f.owner, "")
	raw := text(t, plan)
	if plan.StatusCode != 200 || strings.Contains(raw, "private-fixture") || strings.Contains(raw, "original") {
		t.Fatal(plan.StatusCode, raw)
	}
	var p struct {
		Fingerprint string `json:"fingerprint"`
	}
	if e := json.Unmarshal([]byte(raw), &p); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		fp     string
		rev    int
		status int
	}{{"", 1, 400}, {"wrong", 1, 409}, {p.Fingerprint, 2, 409}, {p.Fingerprint, 1, 200}} {
		r := controlWrite(t, f, "/control/v1/agents/codex-local/apply", map[string]any{"expected_revision": tc.rev, "fingerprint": tc.fp}, true)
		if r.StatusCode != tc.status {
			t.Fatal(r.StatusCode, text(t, r))
		}
	}
	token, _ := f.store.BindingToken(b.ID)
	for _, action := range []string{"inspect", "plan"} {
		r := request(t, "GET", f.control.URL+"/control/v1/agents/codex-local/"+action, f.owner, "")
		raw := text(t, r)
		if strings.Contains(raw, token) || strings.Contains(raw, "private-fixture") {
			t.Fatal("management leaked config or credential")
		}
	}
	r = controlWrite(t, f, "/control/v1/agents/codex-local/disable", map[string]any{"expected_revision": 2}, true)
	if r.StatusCode != 409 {
		t.Fatal(text(t, r))
	}
	still, _ := os.ReadFile(path)
	if !strings.Contains(string(still), "anchor_codex_local") {
		t.Fatal("stale disable restored config")
	}
	r = controlWrite(t, f, "/control/v1/agents/codex-local/disable", map[string]any{"expected_revision": 1}, true)
	if r.StatusCode != 200 {
		t.Fatal(text(t, r))
	}
	restored, _ := os.ReadFile(path)
	if !strings.Contains(string(restored), `model = "original"`) || strings.Contains(string(restored), "anchor_codex_local") {
		t.Fatal(string(restored))
	}
}

func TestMultiModelAuthorizationAndMigrationAPI(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if input["model"] != "actual" {
			t.Error(input)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"response","object":"response","status":"completed","output":[]}`))
	}))
	defer upstream.Close()
	f.add(t, "on", upstream.URL, "secret")
	f.add(t, "off", upstream.URL, "secret")
	p, _ := f.store.Provider("off")
	p.Enabled = false
	if _, err := f.store.PutProvider(p, p.Revision, nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../state/testdata/codex-models.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, provider, version   string
		tools, stream, metadata bool
	}{
		{"default", "on", "0.159.3", true, true, true}, {"alternative", "on", "0.159.3", true, true, true},
		{"disabled", "off", "0.159.3", true, true, true}, {"wrong-version", "on", "0.158.0", true, true, true},
		{"no-tools", "on", "0.159.3", false, true, true}, {"no-stream", "on", "0.159.3", true, false, true},
		{"no-metadata", "on", "0.159.3", true, true, false},
	} {
		m := state.Model{ID: tc.id, Tools: tc.tools, Streaming: tc.stream, Candidates: []state.Candidate{{ProviderID: tc.provider, Model: "actual"}}}
		if tc.metadata {
			m.Codex, err = state.ImportCodexMetadata(raw, "fixture-source", tc.version)
			if err != nil {
				t.Fatal(err)
			}
		}
		if _, err = f.store.PutModel(m, 0); err != nil {
			t.Fatal(err)
		}
	}
	exe, _ := os.Executable()
	b, err := f.store.AddBinding(state.Binding{ID: "multi", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: t.TempDir(), Executable: exe, Model: "default", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	token, _ := f.store.BindingToken(b.ID)
	for _, tc := range []struct {
		scope       string
		rev, status int
	}{
		{"", 1, 400}, {"invalid", 1, 400}, {"all-compatible", 2, 409}, {"all-compatible", 1, 200},
	} {
		resp := controlWrite(t, f, "/control/v1/agents/multi/set-models", map[string]any{"expected_revision": tc.rev, "model": "default", "model_scope": tc.scope}, true)
		if resp.StatusCode != tc.status {
			t.Fatal(resp.StatusCode, text(t, resp))
		}
	}
	resp := request(t, "GET", f.gateway.URL+"/v1/models", token, "")
	body := text(t, resp)
	if resp.StatusCode != 200 || !strings.Contains(body, `"id":"alternative"`) || strings.Contains(body, "no-metadata") || strings.Contains(body, "base_instructions") || strings.Contains(body, "actual") {
		t.Fatal(body)
	}
	for _, id := range []string{"default", "alternative", "disabled", "wrong-version", "no-tools", "no-stream", "no-metadata", "unknown"} {
		resp := request(t, "POST", f.gateway.URL+"/v1/responses", token, `{"model":"`+id+`","input":"mock"}`)
		want := 403
		if id == "default" || id == "alternative" {
			want = 200
		} else if id == "disabled" {
			want = 503
		} else if id == "unknown" {
			want = 404
		}
		if resp.StatusCode != want {
			t.Fatal(id, resp.StatusCode, text(t, resp))
		}
	}
	if calls.Load() != 2 {
		t.Fatal("denied request reached upstream", calls.Load())
	}
	if resp = request(t, "GET", f.control.URL+"/control/v1/status", token, ""); resp.StatusCode != 401 {
		t.Fatal("binding credential managed core")
	}
	p, _ = f.store.Provider("on")
	p.Enabled = false
	if _, err = f.store.PutProvider(p, p.Revision, nil); err != nil {
		t.Fatal(err)
	}
	if resp = request(t, "POST", f.gateway.URL+"/v1/responses", token, `{"model":"alternative"}`); resp.StatusCode != 503 {
		t.Fatal(resp.StatusCode, text(t, resp))
	}
	if _, err = f.store.EnableBinding(b.ID, 2, false); err != nil {
		t.Fatal(err)
	}
	if resp = request(t, "GET", f.gateway.URL+"/v1/models", token, ""); resp.StatusCode != 401 {
		t.Fatal("disabled credential accepted")
	}
}
