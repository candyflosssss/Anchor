package desktop

import "sync/atomic"

// WindowLifecycle separates closing to the tray from an explicit host exit.
// It does not own or stop the detached core.
type WindowLifecycle struct{ exiting atomic.Bool }

func (w *WindowLifecycle) RequestExit()  { w.exiting.Store(true) }
func (w *WindowLifecycle) Exiting() bool { return w.exiting.Load() }

// BeforeClose returns true only when a recoverable hide replaces a close.
// Without a usable tray, let the host exit so no invisible window is stranded.
func (w *WindowLifecycle) BeforeClose(trayAvailable bool, hide func()) bool {
	if w.Exiting() || !trayAvailable {
		return false
	}
	hide()
	return true
}
