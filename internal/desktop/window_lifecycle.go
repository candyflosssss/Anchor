package desktop

import (
	"sync"
	"sync/atomic"
)

// WindowLifecycle separates closing to the tray from an explicit host exit.
// It does not own or stop the detached core.
type WindowLifecycle struct {
	exiting     atomic.Bool
	mu          sync.Mutex
	show        func()
	pendingShow bool
}

// Ready installs the native restore/show operation once Wails has a window.
// Multiple launches during startup coalesce into one restore request.
func (w *WindowLifecycle) Ready(show func()) {
	w.mu.Lock()
	w.show = show
	pending := w.pendingShow
	w.pendingShow = false
	w.mu.Unlock()
	if pending && !w.Exiting() {
		show()
	}
}

// Show is shared by repeated launches and the tray. An early request is queued;
// explicit exit or update handoff rejects activation.
func (w *WindowLifecycle) Show() bool {
	w.mu.Lock()
	if w.Exiting() {
		w.mu.Unlock()
		return false
	}
	show := w.show
	if show == nil {
		w.pendingShow = true
	}
	w.mu.Unlock()
	if show != nil && !w.Exiting() {
		show()
	}
	return true
}

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
