package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestAPIServiceBridgeAndCLI(t *testing.T) {
	s := testSession(t)
	defer s.Close()
	b := NewBridge(s)
	requireReady(t, b)
	v := b.APIService()
	if v.Problem != nil || v.Data == nil || v.Data.Settings.Enabled || v.Data.RestartRequired {
		t.Fatal(v)
	}
	no, yes := false, true
	saved := b.SaveAPIService(SaveAPIServiceSettings{Enabled: &no, Port: 17863, AllowLAN: &yes, ExpectedRevision: v.Data.Settings.Revision})
	if saved.Problem != nil || saved.Data == nil || saved.Data.Settings.Port != 17863 || saved.Data.Settings.Revision != 2 {
		t.Fatal(saved)
	}
	if v := b.SaveAPIService(SaveAPIServiceSettings{Enabled: &no, Port: 17863, AllowLAN: &yes, ExpectedRevision: 1}); v.Problem == nil || v.Problem.Code != "conflict" {
		t.Fatal(v)
	}
	issued := b.IssueAPIKey(IssueAPIKey{Label: "phone", TTLSeconds: 3600})
	if issued.Problem != nil || issued.Data == nil || issued.Data.Token == "" || issued.Data.Access.Scope != "gateway" {
		t.Fatal("Key issue failed")
	}
	listed := b.APIService()
	raw, _ := json.Marshal(listed)
	if listed.Problem != nil || strings.Contains(string(raw), issued.Data.Token) || len(listed.Data.Keys) != 1 {
		t.Fatal("Key leaked or list failed")
	}
	revoked := b.RevokeAPIKey(RevokeAPIKey{ID: issued.Data.Access.ID})
	if revoked.Problem != nil || revoked.Data == nil || !revoked.Data.Keys[0].Revoked {
		t.Fatal(revoked)
	}
	run := func(wantErr bool, args ...string) string {
		t.Helper()
		var output bytes.Buffer
		err := (cli.App{Out: &output, Err: io.Discard, In: strings.NewReader("")}).Run(context.Background(), append([]string{"--data-dir", s.options.Dir}, args...))
		if (err != nil) != wantErr {
			t.Fatal(args, err)
		}
		return output.String()
	}
	run(false, "context", "add", "local-api", "--local")
	run(false, "context", "use", "local-api")
	run(false, "api", "settings")
	run(true, "api", "issue", "--label", "unapproved")
	var key state.IssuedAPIKey
	if err := json.Unmarshal([]byte(run(false, "api", "issue", "--label", "editor", "--show-once")), &key); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(run(false, "api", "keys"), key.Token) {
		t.Fatal("secret in metadata")
	}
	run(true, "api", "revoke", "--id", key.Access.ID)
	run(false, "api", "revoke", "--id", key.Access.ID, "--approve")
	run(true, "api", "configure", "--revision", "2")
	if out := run(false, "api", "configure", "--enabled=true", "--port", "17864", "--allow-lan=true", "--revision", "2", "--approve"); !strings.Contains(out, `"restart_required":true`) {
		t.Fatal(out)
	}
	run(true, "api", "configure", "--enabled=true", "--port", "17864", "--revision", "2", "--approve")
	for _, args := range [][]string{{"logs", "list", "--access-key", key.Access.ID}, {"logs", "stats", "--group-by", "access_key"}, {"logs", "suggestions", "--field", "access_key"}, {"logs", "export", "--access-key", key.Access.ID}} {
		run(false, args...)
	}
}
func TestAPIServiceSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/api-service.json")
	if err != nil {
		t.Fatal(err)
	}
	var v APIServiceWorkspace
	if err = json.Unmarshal(raw, &v); err != nil || v.Settings.Port != 17862 || v.Settings.Enabled || v.Settings.AllowLAN || v.RestartRequired || v.ActiveAddress != "" || v.Keys == nil || v.BaseURLs == nil {
		t.Fatal(v, err)
	}
}

func TestAPIServiceUnsupportedProblem(t *testing.T) {
	for _, writing := range []bool{false, true} {
		got := problem(&client.RequestError{Status: 404, Code: "unsupported"}, writing)
		if got == nil || got.Code != "unsupported" {
			t.Fatal(got)
		}
	}
}
