package desktop

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestCCSwitchBridgeCLIAndIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dbDir := filepath.Join(home, ".cc-switch")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dbDir, "cc-switch.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE providers(id TEXT,app_type TEXT,name TEXT,settings_config TEXT,website_url TEXT,meta TEXT,is_current INTEGER);
INSERT INTO providers VALUES('one','claude','测试','{"env":{"ANTHROPIC_BASE_URL":"https://upstream.invalid","ANTHROPIC_AUTH_TOKEN":"synthetic-private","ANTHROPIC_MODEL":"upstream-model"}}','','{}',0);`)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	st, err := state.Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := core.New(st, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer srv.Close()
	web := httptest.NewServer(srv.Control())
	defer web.Close()
	cfg, err := st.Initialize(strings.TrimPrefix(web.URL, "http://"), "127.0.0.1:17860")
	if err != nil {
		t.Fatal(err)
	}
	session := NewSession(Options{Dir: dir})
	defer session.Close()
	session.cfg = &cfg
	b := NewBridge(session)
	preview := b.ImportCCSwitch(CCImportRequest{})
	if preview.Problem != nil || preview.Data == nil || len(preview.Data.Providers) != 1 || preview.Data.Applied {
		t.Fatalf("%+v", preview)
	}
	encoded, _ := json.Marshal(preview)
	if strings.Contains(string(encoded), "synthetic-private") {
		t.Fatal("secret leaked through bridge")
	}
	providers, _ := st.Providers()
	if len(providers) != 0 {
		t.Fatal("preview wrote target")
	}
	invalid := b.ImportCCSwitch(CCImportRequest{Approved: true})
	if invalid.Problem == nil || invalid.Problem.Code != "invalid" {
		t.Fatal(invalid)
	}
	stale := b.ImportCCSwitch(CCImportRequest{Approved: true, Fingerprint: "stale"})
	if stale.Problem == nil || stale.Problem.Code != "conflict" {
		t.Fatal(stale)
	}
	// CLI previews and applies the same reviewed contract through the real
	// authenticated HTTP client, while the desktop reads the resulting state.
	run := func(wantErr bool, args ...string) string {
		t.Helper()
		var out bytes.Buffer
		a := cli.App{Out: &out, Err: io.Discard, In: strings.NewReader("")}
		err := a.Run(context.Background(), append([]string{"--data-dir", dir}, args...))
		if (err != nil) != wantErr {
			t.Fatalf("CLI: %v %s", err, out.String())
		}
		return out.String()
	}
	run(false, "context", "add", "local", "--local")
	run(false, "context", "use", "local")
	cliPreview := run(false, "provider", "import-ccs")
	var report CCImportReport
	if json.Unmarshal([]byte(cliPreview), &report) != nil || report.Fingerprint != preview.Data.Fingerprint {
		t.Fatal("CLI / bridge contract drift")
	}
	run(true, "provider", "import-ccs", "--approve")
	run(true, "provider", "import-ccs", "--fingerprint", report.Fingerprint)
	run(false, "provider", "import-ccs", "--approve", "--fingerprint", report.Fingerprint)
	rows := b.Providers()
	if rows.Problem != nil || len(rows.Data) != 1 || !rows.Data[0].Enabled {
		t.Fatal(rows)
	}
	repeat := b.ImportCCSwitch(CCImportRequest{Approved: true, Fingerprint: report.Fingerprint})
	if repeat.Problem == nil || repeat.Problem.Code != "conflict" {
		t.Fatal("uncertain write could replay")
	}
	next := b.ImportCCSwitch(CCImportRequest{})
	if next.Problem != nil || next.Data.Providers[0].Status != "exists" {
		t.Fatal(next)
	}
	run(true, "provider", "import-ccs", "--db", filepath.Join(home, "missing.db"))
	// Owner authorization and selected-instance identity are enforced for reads.
	owner, err := st.OwnerToken()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ token, instance string }{{"", cfg.InstanceID}, {owner, "wrong-instance"}} {
		req, _ := http.NewRequest("POST", web.URL+"/control/v1/import/cc-switch", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("X-Anchor-Instance", tc.instance)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Fatal("unbound source read accepted")
		}
	}
}

func TestCCSwitchSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/ccswitch-import.json")
	if err != nil {
		t.Fatal(err)
	}
	var report CCImportReport
	if err = json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Fingerprint) != 64 || report.Applied || len(report.Providers) != 1 || len(report.Providers[0].Keys) != 2 || len(report.Models) != 1 || report.Issues[0].Reason != "unsupported_protocol" {
		t.Fatal("fixture semantics drift")
	}
	encoded, _ := json.Marshal(report)
	var before, after any
	_ = json.Unmarshal(raw, &before)
	_ = json.Unmarshal(encoded, &after)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("Go / TypeScript fixture shape drift")
	}
}
