package core

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestGatewayLogAttribution(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Anchor-Project") != "" || r.Header.Get("X-Anchor-Session") != "" {
			t.Error("local attribution forwarded upstream")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","status":"completed","output":[],"usage":{"input_tokens":12,"output_tokens":7}}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "fixture-provider-secret")
	f.model(t, state.Candidate{ProviderID: "p", Model: "upstream"})
	exe, _ := os.Executable()
	b, err := f.store.AddBinding(state.Binding{ID: "desktop", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: t.TempDir(), Model: "public", Executable: exe, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.store.BindingToken(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	request := func(auth, project, session, body string) logbook.Request {
		t.Helper()
		r, _ := http.NewRequest("POST", f.gateway.URL+"/v1/responses", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+auth)
		r.Header.Set("X-Anchor-Project", project)
		r.Header.Set("X-Anchor-Session", session)
		r.Header.Set("X-Anchor-Agent", "forged-agent")
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if err = f.core.logs.Flush(t.Context()); err != nil {
			t.Fatal(err)
		}
		row, err := f.core.logs.Detail(t.Context(), response.Header.Get("X-Anchor-Request-ID"))
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	body := `{"model":"public","input":"DO_NOT_STORE"}`
	row := request(token, "anchor", "thread-1", body)
	if row.Agent != "codex" || row.Binding != "desktop" || row.Project != "anchor" || row.Session != "thread-1" {
		t.Fatal(row)
	}
	failed := request(token, "anchor", "thread-2", "{")
	if failed.Agent != "codex" || failed.Status != "failed" {
		t.Fatal(failed)
	}
	general := request(f.token, "/private/home/project", "bad session", body)
	if general.Agent != "" || general.Binding != "" || general.Project != "" || general.Session != "" {
		t.Fatal(general)
	}
	c := logClient(t, f)
	stats, err := c.LogStats(t.Context(), logbook.StatsQuery{Filter: logbook.Query{Agent: &row.Agent, Project: &row.Project}, GroupBy: "session"})
	if err != nil || stats.Summary.Requests != 2 || stats.GroupCount != 2 {
		t.Fatal(stats, err)
	}
	var exported bytes.Buffer
	if err = c.ExportLogs(t.Context(), logbook.Query{Session: &row.Session}, &exported); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exported.String(), `"session":"thread-1"`) || strings.Contains(exported.String(), "thread-2") || strings.Contains(exported.String(), "DO_NOT_STORE") || strings.Contains(exported.String(), token) {
		t.Fatal("export attribution or privacy mismatch")
	}
}

func TestLogIdentityValidation(t *testing.T) {
	for _, v := range [][]string{nil, {""}, {"one", "two"}, {"/home/private"}, {"C:\\private"}, {"a b"}, {strings.Repeat("a", 129)}, {"-x"}, {"a,b"}, {"会话"}} {
		if got := logIdentity(v); got != "" {
			t.Fatalf("accepted %q", got)
		}
	}
	if logIdentity([]string{"credential"}, "credential") != "" {
		t.Fatal("credential recorded as identity")
	}
	for _, v := range []string{"project", "thread_123", "uuid-123:4", "Repo.Name@branch"} {
		if logIdentity([]string{v}) != v {
			t.Fatal(v)
		}
	}
}
