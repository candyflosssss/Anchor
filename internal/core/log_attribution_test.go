package core

import (
	"bytes"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/state"
)

const nativeThread = "01a14453-be7d-7881-afc3-dc082aa5c529"
const parentThread = "01a14453-be7d-7881-afc3-dc082aa5c520"

func TestCodexLogSession(t *testing.T) {
	for _, tc := range []struct {
		name, metadata, legacy, want string
	}{
		{"current", `{"thread_id":"` + nativeThread + `","session_id":"` + parentThread + `","turn_id":"not-a-session"}`, "", nativeThread},
		{"metadata_session", `{"session_id":"` + nativeThread + `"}`, "", nativeThread},
		{"legacy", "", nativeThread, nativeThread},
		{"case", `{"thread_id":"` + strings.ToUpper(nativeThread) + `"}`, "", nativeThread},
		{"not_turn", `{"turn_id":"` + nativeThread + `","window_id":"` + nativeThread + `"}`, "", ""},
		{"not_prompt_cache", `{"prompt_cache_key":"` + nativeThread + `"}`, "", ""},
		{"invalid", `{"thread_id":"/home/private"}`, "", ""},
		{"malformed", `{"thread_id":`, "", ""},
		{"oversized", strings.Repeat("x", 8193), "", ""},
		{"malformed_uuid", `{"thread_id":"` + strings.ReplaceAll(nativeThread, "-", "x") + `"}`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.metadata != "" {
				h.Set("x-codex-turn-metadata", tc.metadata)
			}
			if tc.legacy != "" {
				h.Set("session_id", tc.legacy)
			}
			if got := codexLogSession(h); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	h := http.Header{}
	h.Add("session_id", nativeThread)
	h.Add("session_id", parentThread)
	h.Add("x-codex-turn-metadata", `{"thread_id":"`+nativeThread+`"}`)
	h.Add("x-codex-turn-metadata", `{"thread_id":"`+parentThread+`"}`)
	if codexLogSession(h) != "" {
		t.Fatal("accepted ambiguous metadata")
	}
	h = http.Header{}
	h.Set("session_id", nativeThread)
	if codexLogSession(h, nativeThread) != "" {
		t.Fatal("recorded credential")
	}
}

func TestGatewayCodexNativeLogAttribution(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, header := range []string{"X-Anchor-Project", "X-Anchor-Session", "x-codex-turn-metadata", "session_id"} {
			if r.Header.Get(header) != "" {
				t.Errorf("unexpected upstream header %s", header)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","status":"completed","output":[],"usage":{"input_tokens":12,"output_tokens":7}}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "fixture-provider-secret")
	f.model(t, state.Candidate{ProviderID: "p", Model: "upstream"})
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "state_5.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(root, "private", "Anchor")
	_, err = db.Exec(`CREATE TABLE threads(id TEXT PRIMARY KEY,cwd TEXT); INSERT INTO threads VALUES(?,?)`, nativeThread, cwd)
	db.Close()
	if chmodErr := os.Chmod(filepath.Join(root, "state_5.sqlite"), 0600); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	tokens := map[string]string{"global": f.token}
	for _, adapter := range []string{"codex", "claude"} {
		configRoot := root
		if adapter != "codex" {
			configRoot = t.TempDir()
		}
		b, err := f.store.AddBinding(state.Binding{ID: adapter, AdapterID: adapter, AdapterVersion: "1", ConfigRoot: configRoot, Model: "public", Executable: exe, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		tokens[adapter], err = f.store.BindingToken(b.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	var automatic logbook.Request
	for _, tc := range []struct{ auth, project, session, body string }{
		{"codex", "", "", `{"model":"public","input":"DO_NOT_STORE"}`},
		{"codex", "", "", "{"},
		{"codex", "manual-project", "manual-session", `{"model":"public","input":"x"}`},
		{"global", "", "", `{"model":"public","input":"x"}`},
		{"claude", "", "", `{"model":"public","input":"x"}`},
	} {
		r, _ := http.NewRequest("POST", f.gateway.URL+"/v1/responses", strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer "+tokens[tc.auth])
		r.Header.Set("x-codex-turn-metadata", `{"thread_id":"`+nativeThread+`","session_id":"`+parentThread+`","installation_id":"DO_NOT_STORE_INSTALLATION"}`)
		if tc.project != "" {
			r.Header.Set("X-Anchor-Project", tc.project)
			r.Header.Set("X-Anchor-Session", tc.session)
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if err := f.core.logs.Flush(t.Context()); err != nil {
			t.Fatal(err)
		}
		row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
		if err != nil {
			t.Fatal(err)
		}
		if tc.auth == "codex" && tc.project == "" {
			if row.Session != nativeThread || !strings.HasPrefix(row.Project, "Anchor@") || row.Agent != "codex" {
				t.Fatalf("native attribution missing: %+v", row)
			}
			automatic = row
		} else if row.Project != tc.project || row.Session != tc.session {
			t.Fatal("native metadata crossed binding boundary or overrode explicit labels")
		}
	}
	c := logClient(t, f)
	stats, err := c.LogStats(t.Context(), logbook.StatsQuery{Filter: logbook.Query{Project: &automatic.Project}, GroupBy: "session"})
	if err != nil || stats.Summary.Requests != 2 || len(stats.Groups) != 1 || stats.Groups[0].Value != nativeThread {
		t.Fatal(stats, err)
	}
	for field, prefix := range map[string]string{"project": "Anchor", "session": "01a14453"} {
		suggestions, err := c.LogSuggestions(t.Context(), logbook.SuggestionsQuery{Kind: "requests", Field: field, Prefix: prefix})
		if err != nil || len(suggestions.Items) != 1 || suggestions.Items[0].Count != 2 {
			t.Fatal(suggestions, err)
		}
	}
	var exported bytes.Buffer
	if err := c.ExportLogs(t.Context(), logbook.Query{Session: &automatic.Session}, &exported); err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{cwd, "DO_NOT_STORE", tokens["codex"], "installation_id"} {
		if strings.Contains(exported.String(), private) {
			t.Fatal("private context exported")
		}
	}
}
