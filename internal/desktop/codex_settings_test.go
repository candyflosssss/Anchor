package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/cli"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCodexSettingsBridgeAndCLI(t *testing.T) {
	s := testSession(t)
	defer s.Close()
	b := NewBridge(s)
	requireReady(t, b)
	initial := b.CodexSettings()
	if initial.Problem != nil || initial.Data == nil || initial.Data.Settings.Retries != 5 {
		t.Fatal(initial)
	}
	n := 0
	in := SaveCodexSettings{Retries: &n, ExpectedRevision: initial.Data.Settings.Revision}
	out := b.SaveCodexSettings(in)
	if out.Problem != nil || out.Data == nil || out.Data.Settings.Retries != 0 || out.Data.Settings.Revision != 2 {
		t.Fatal(out)
	}
	if stale := b.SaveCodexSettings(in); stale.Problem == nil || stale.Problem.Code != "conflict" {
		t.Fatal(stale)
	}
	n = 11
	if invalid := b.SaveCodexSettings(in); invalid.Problem == nil || invalid.Problem.Code != "invalid" {
		t.Fatal(invalid)
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
	run(false, "context", "add", "local-test", "--local")
	run(false, "context", "use", "local-test")
	if got := run(false, "agent", "codex-settings"); !strings.Contains(got, `"retries":0`) {
		t.Fatal(got)
	}
	if got := run(false, "agent", "codex-settings", "--retries", "3", "--revision", "2"); !strings.Contains(got, `"retries":3`) || !strings.Contains(got, `"revision":3`) {
		t.Fatal(got)
	}
	run(true, "agent", "codex-settings", "--retries", "4", "--revision", "2")
	run(true, "agent", "codex-settings", "--retries", "11", "--revision", "3")
	run(true, "agent", "codex-settings", "--retries", "3")
}
func TestSharedCodexSettingsFixture(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/codex-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var out CodexSettingsState
	if err = json.Unmarshal(raw, &out); err != nil || out.Settings.Retries != 5 || out.Settings.Revision != 1 || out.NeedsSync == nil {
		t.Fatal(out, err)
	}
	encoded, _ := json.Marshal(out)
	var a, b any
	json.Unmarshal(raw, &a)
	json.Unmarshal(encoded, &b)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("fixture drift")
	}
}
