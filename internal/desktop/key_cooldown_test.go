package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/state"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestKeyCooldownBridgeAndCLI(t *testing.T) {
	s := testSession(t)
	defer s.Close()
	b := NewBridge(s)
	requireReady(t, b)
	initial := b.KeyCooldownSettings()
	if initial.Problem != nil || initial.Data == nil || *initial.Data != state.DefaultKeyCooldownSettings() {
		t.Fatal(initial)
	}
	enabled, threshold, seconds, rate := false, 2, 60, 90
	in := SaveKeyCooldownSettings{&enabled, &threshold, &seconds, &rate, 1}
	out := b.SaveKeyCooldownSettings(in)
	if out.Problem != nil || out.Data == nil || out.Data.Enabled || out.Data.Revision != 2 {
		t.Fatal(out)
	}
	if stale := b.SaveKeyCooldownSettings(in); stale.Problem == nil || stale.Problem.Code != "conflict" {
		t.Fatal(stale)
	}
	if invalid := b.SaveKeyCooldownSettings(SaveKeyCooldownSettings{}); invalid.Problem == nil || invalid.Problem.Code != "invalid" {
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
	if got := run(false, "settings", "key-cooldown"); !strings.Contains(got, `"enabled":false`) {
		t.Fatal(got)
	}
	if got := run(false, "settings", "key-cooldown", "--enabled=true", "--revision", "2"); !strings.Contains(got, `"enabled":true`) || !strings.Contains(got, `"cooldown_seconds":60`) || !strings.Contains(got, `"revision":3`) {
		t.Fatal(got)
	}
	run(true, "settings", "key-cooldown", "--cooldown-seconds", "50", "--revision", "2")
	run(true, "settings", "key-cooldown", "--cooldown-seconds", "0", "--revision", "3")
	run(true, "settings", "key-cooldown", "--failure-threshold", "101", "--revision", "3")
	run(true, "settings", "key-cooldown", "--rate-limit-seconds", "3601", "--revision", "3")
	run(true, "settings", "key-cooldown", "--enabled=false")
	run(true, "settings", "key-cooldown", "--revision", "3")
	run(true, "settings", "key-cooldown", "unexpected")
	run(true, "settings", "other")
}
func TestSharedKeyCooldownFixture(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/key-cooldown.json")
	if err != nil {
		t.Fatal(err)
	}
	var out KeyCooldownSettings
	if err = json.Unmarshal(raw, &out); err != nil || out != state.DefaultKeyCooldownSettings() {
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
