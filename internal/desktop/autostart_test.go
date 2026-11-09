package desktop

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/autostart"
	"github.com/candyflosssss/Anchor/internal/connection"
)

type fakeAutostart struct {
	saves  int
	result autostart.Status
	err    error
}

func (f *fakeAutostart) Status() (autostart.Status, error) { return f.result, f.err }
func (f *fakeAutostart) Save(in autostart.Save) (autostart.Status, error) {
	f.saves++
	f.result.Enabled = *in.Enabled
	return f.result, f.err
}
func TestAutostartHostContract(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/autostart.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture AutostartResult
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	s := NewSession(Options{Dir: t.TempDir()})
	defer s.Close()
	f := &fakeAutostart{result: *fixture.Data}
	s.autostartManager = f
	// No remote connection is consulted, nor is an offline core started.
	s.remote = &connection.Live{}
	b := NewBridge(s)
	got := b.Autostart()
	if got.Problem != nil || *got.Data != *fixture.Data {
		t.Fatalf("%+v", got)
	}
	enabled := true
	in := SaveAutostart{Enabled: &enabled, ExpectedRevision: strings.Repeat("a", 64), Approved: true}
	if got = b.SaveAutostart(in); got.Problem != nil || !got.Data.Enabled || f.saves != 1 {
		t.Fatalf("%+v", got)
	}
	s.updating = true
	if got = b.SaveAutostart(in); got.Problem == nil || got.Problem.Code != "conflict" || f.saves != 1 {
		t.Fatalf("%+v", got)
	}
	s.updating = false
	s.remote = nil
	s.Close()
	if got = b.Autostart(); got.Problem == nil || got.Problem.Code != "closed" {
		t.Fatalf("%+v", got)
	}
}
func TestAutostartErrors(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{{autostart.ErrConflict, "conflict"}, {autostart.ErrInvalid, "invalid"}, {autostart.ErrUnsupported, "unsupported"}, {autostart.ErrUnknown, "unknown"}, {os.ErrPermission, "permission"}} {
		s := NewSession(Options{})
		s.autostartManager = &fakeAutostart{err: test.err}
		b := NewBridge(s)
		enabled := true
		got := b.SaveAutostart(SaveAutostart{&enabled, strings.Repeat("a", 64), true})
		s.Close()
		if got.Problem == nil || got.Problem.Code != test.code {
			t.Fatalf("%v: %+v", test.err, got)
		}
	}
	if !IsCLIInvocation([]string{"--data-dir", "x", "autostart", "status"}) {
		t.Fatal("headless desktop autostart not recognized")
	}
}
