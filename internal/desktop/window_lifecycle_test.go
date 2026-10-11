package desktop

import "testing"

func TestWindowCloseLifecycle(t *testing.T) {
	for _, tt := range []struct {
		name                string
		tray, exiting, hide bool
	}{
		{"close hides in a usable tray", true, false, true},
		{"missing tray lets the host close", false, false, false},
		{"explicit exit bypasses tray", true, true, false},
		{"update exit also works without tray", false, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var w WindowLifecycle
			if tt.exiting {
				w.RequestExit()
			}
			hidden := false
			preventClose := w.BeforeClose(tt.tray, func() { hidden = true })
			if hidden != tt.hide || preventClose != tt.hide {
				t.Fatalf("hidden=%v preventClose=%v, want both %v", hidden, preventClose, tt.hide)
			}
		})
	}
}

func TestExitDesktopUsesHostQuitWithoutCore(t *testing.T) {
	var window WindowLifecycle
	session := NewSession(Options{Dir: t.TempDir(), Quit: window.RequestExit})
	defer session.Close()
	bridge := NewBridge(session)
	out := bridge.ExitDesktop()
	if out.Problem != nil || out.Data == nil || !out.Data.Accepted || !window.Exiting() {
		t.Fatalf("exit did not reach host: %#v", out)
	}
	if window.BeforeClose(true, func() { t.Fatal("explicit exit must not hide") }) {
		t.Fatal("explicit exit was blocked by the tray")
	}
}

func TestExitDesktopCannotInterruptUpdateHandoff(t *testing.T) {
	session := NewSession(Options{Dir: t.TempDir(), Quit: func() { t.Fatal("exit interrupted update handoff") }})
	defer session.Close()
	session.updating = true
	out := NewBridge(session).ExitDesktop()
	if out.Problem == nil || out.Problem.Code != "conflict" || out.Data != nil {
		t.Fatalf("expected conflict during update handoff: %#v", out)
	}
}

func TestWindowActivationLifecycle(t *testing.T) {
	var window WindowLifecycle
	if !window.Show() || !window.Show() {
		t.Fatal("startup launches must queue activation")
	}
	shows := 0
	window.Ready(func() { shows++ })
	if shows != 1 {
		t.Fatalf("pending launches must coalesce: shows=%d", shows)
	}
	window.BeforeClose(true, func() {})
	if !window.Show() || shows != 2 {
		t.Fatal("hidden tray window did not accept activation")
	}
	window.RequestExit()
	if window.Show() || shows != 2 {
		t.Fatal("exiting window accepted activation")
	}
}

func TestWindowExitDiscardsPendingActivation(t *testing.T) {
	var window WindowLifecycle
	window.Show()
	window.RequestExit()
	window.Ready(func() { t.Fatal("exit must discard early activation") })
}
