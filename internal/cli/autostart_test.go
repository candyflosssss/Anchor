package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/autostart"
)

type fakeAutostart struct {
	saves int
	input autostart.Save
}

func (f *fakeAutostart) Status() (autostart.Status, error) {
	return autostart.Status{Supported: true, Revision: strings.Repeat("a", 64)}, nil
}
func (f *fakeAutostart) Save(in autostart.Save) (autostart.Status, error) {
	f.saves++
	f.input = in
	return autostart.Status{Supported: true, Enabled: *in.Enabled}, nil
}
func TestAutostartLocalOfflineCLI(t *testing.T) {
	f := &fakeAutostart{}
	var out bytes.Buffer
	a := App{Out: &out, Err: &out, autostartManager: f}
	base := []string{"--data-dir", t.TempDir()}
	for _, args := range [][]string{{"autostart", "status"}, {"autostart", "enable", "--approve", "--revision", strings.Repeat("a", 64)}, {"autostart", "disable", "--approve", "--revision", strings.Repeat("a", 64)}} {
		if err := a.Run(context.Background(), append(base, args...)); err != nil {
			t.Fatal(err)
		}
	}
	if f.saves != 2 || *f.input.Enabled {
		t.Fatalf("saves: %+v", f)
	}
	for _, args := range [][]string{{"--context", "remote", "autostart", "status"}, {"autostart", "enable"}, {"autostart", "disable", "--approve"}, {"autostart", "status", "--approve"}, {"autostart", "wat"}} {
		if err := a.Run(context.Background(), append(base, args...)); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if f.saves != 2 {
		t.Fatal("invalid operation wrote")
	}
}
