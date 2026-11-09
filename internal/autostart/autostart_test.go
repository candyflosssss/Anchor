package autostart

import (
	"errors"
	"os"
	"strings"
	"testing"
)

type fakeBackend struct {
	s                 snapshot
	writes            int
	readErr, writeErr error
	loseWrite         bool
}

func (b *fakeBackend) read() (snapshot, error) { return b.s, b.readErr }
func (b *fakeBackend) write(command string) error {
	b.writes++
	if b.writeErr != nil {
		return b.writeErr
	}
	if !b.loseWrite {
		b.s.Command = command
	}
	return nil
}
func testManager() (*Manager, *fakeBackend) {
	b := &fakeBackend{}
	m := &Manager{store: b, commandOverride: func() (string, error) {
		return windowsCommand(`C:\Program Files\Anchor\anchor-desktop.exe`, `C:\Users\测试\anchor`)
	}, lockOverride: func() (func(), error) { return func() {}, nil }}
	return m, b
}
func TestDefaultEnableDisableAndCAS(t *testing.T) {
	m, b := testManager()
	initial, err := m.Status()
	if err != nil || initial.Enabled || !initial.Supported || !initial.CanEnable || initial.MatchesCurrent || len(initial.Revision) != 64 {
		t.Fatalf("default: %+v %v", initial, err)
	}
	enabled := true
	in := Save{&enabled, initial.Revision, true}
	out, err := m.Save(in)
	if err != nil || !out.Enabled || !out.MatchesCurrent || b.writes != 1 {
		t.Fatalf("enable: %+v %v", out, err)
	}
	if _, err = m.Save(in); !errors.Is(err, ErrConflict) || b.writes != 1 {
		t.Fatalf("stale write: %v", err)
	}
	in.ExpectedRevision = out.Revision
	if _, err = m.Save(in); err != nil || b.writes != 1 {
		t.Fatalf("idempotent: %v", err)
	}
	enabled = false
	out, err = m.Save(in)
	if err != nil || out.Enabled || b.s.Command != "" || b.writes != 2 {
		t.Fatalf("disable: %+v %v", out, err)
	}
}
func TestApprovalValidationAndFailures(t *testing.T) {
	for _, mode := range []string{"approval", "revision", "read", "permission", "write", "lost", "unavailable", "lock"} {
		t.Run(mode, func(t *testing.T) {
			m, b := testManager()
			current, _ := m.Status()
			enabled := true
			in := Save{&enabled, current.Revision, true}
			expected := ErrInvalid
			switch mode {
			case "approval":
				in.Approved = false
			case "revision":
				in.ExpectedRevision = "not-a-revision"
			case "read":
				b.readErr = os.ErrPermission
				expected = os.ErrPermission
			case "permission":
				b.writeErr = os.ErrPermission
				expected = os.ErrPermission
			case "write":
				b.writeErr = errors.New("broken")
				expected = ErrUnknown
			case "lost":
				b.loseWrite = true
				expected = ErrUnknown
			case "unavailable":
				m.commandOverride = func() (string, error) { return "", ErrInvalid }
			case "lock":
				m.lockOverride = func() (func(), error) { return nil, ErrConflict }
				expected = ErrConflict
			}
			if _, err := m.Save(in); !errors.Is(err, expected) {
				t.Fatalf("got %v, want %v", err, expected)
			}
		})
	}
}
func TestStaleRegistrationAndSystemApproval(t *testing.T) {
	m, b := testManager()
	b.s = snapshot{Command: `"C:\old.exe" --core`, Approval: []byte{3, 0, 0, 0}}
	out, _ := m.Status()
	if !out.Enabled || out.MatchesCurrent || !out.DisabledBySystem {
		t.Fatalf("stale: %+v", out)
	}
	before := out.Revision
	b.s.Approval[0] = 2
	out, _ = m.Status()
	if out.DisabledBySystem || out.Revision == before {
		t.Fatalf("approval revision: %+v", out)
	}
	// Removal remains possible if the executable or data directory was deleted.
	m.commandOverride = func() (string, error) { return "", ErrInvalid }
	enabled := false
	out, err := m.Save(Save{&enabled, out.Revision, true})
	if err != nil || out.Enabled {
		t.Fatalf("remove stale: %+v %v", out, err)
	}
}
func TestUnsupportedDoesNotWrite(t *testing.T) {
	m := &Manager{}
	out, err := m.Status()
	if err != nil || out != (Status{}) {
		t.Fatalf("unsupported: %+v %v", out, err)
	}
	enabled := true
	if _, err = m.Save(Save{&enabled, strings.Repeat("0", 64), true}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}
func TestWindowsCommand(t *testing.T) {
	got, err := windowsCommand(`D:\应用程序\Anchor Desktop\anchor-desktop.exe`, `C:\Users\Name With Spaces\anchor\`)
	want := `"D:\应用程序\Anchor Desktop\anchor-desktop.exe" --data-dir "C:\Users\Name With Spaces\anchor\\" --core`
	if err != nil || got != want {
		t.Fatalf("%q %v", got, err)
	}
	if got := quote(`a\"b`); got != `"a\\\"b"` {
		t.Fatalf("quote: %q", got)
	}
	for _, bad := range []string{`relative`, `\\server\share`, `C:relative`, "C:\\bad\npath", `C:\bad"path`, `C:\` + strings.Repeat("长", 260)} {
		if _, err := windowsCommand(bad, `C:\data`); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %q", bad)
		}
	}
}
