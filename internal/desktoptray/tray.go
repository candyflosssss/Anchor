//go:build desktop && (windows || linux)

// Package desktoptray owns the native notification-area entry, independently
// from the Wails UI loop and the detached core.
package desktoptray

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"
)

// Tray is started once by the GUI host. Core/helper modes never create it.
type Tray struct {
	ready   atomic.Bool
	mu      sync.Mutex
	stop    chan struct{}
	done    chan struct{}
	stopped bool
}

func (t *Tray) Start(show, exit, exitWithCore func()) {
	t.mu.Lock()
	if t.stop != nil || t.stopped || !platformSupported() {
		t.mu.Unlock()
		return
	}
	t.stop = make(chan struct{})
	t.done = make(chan struct{})
	stop, done := t.stop, t.done
	t.mu.Unlock()
	go func() {
		defer close(done)
		// Windows creates its hidden tray window on this thread; its message
		// loop must stay on the same thread. Wails keeps its own GUI thread.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer t.ready.Store(false)
		systray.Run(func() {
			systray.SetIcon(icon())
			systray.SetTooltip("Anchor · 点击打开，右键选择退出方式")
			systray.SetOnTapped(show)
			openItem := systray.AddMenuItem("打开 Anchor", "恢复主窗口")
			systray.AddSeparator()
			exitItem := systray.AddMenuItem("退出界面（核心继续运行）", "仅退出窗口和托盘，不停止核心")
			exitWithCoreItem := systray.AddMenuItem("退出 Anchor（同时停止本机核心）", "确认后正常停止本机核心，再退出窗口和托盘；不停止远程核心")
			t.ready.Store(true)
			go func() {
				for {
					select {
					case <-openItem.ClickedCh:
						show()
					case <-exitItem.ClickedCh:
						exit()
					case <-exitWithCoreItem.ClickedCh:
						exitWithCore()
					case <-stop:
						systray.Quit()
						return
					}
				}
			}()
		}, nil)
	}()
}

func (t *Tray) Available() bool { return t.ready.Load() && platformAvailable() }

func (t *Tray) Stop() {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.stopped = true
	if t.stop == nil {
		t.mu.Unlock()
		return
	}
	close(t.stop)
	done := t.done
	t.mu.Unlock()
	// Let the native loop remove the icon before the host exits. Never block
	// an approved update handoff indefinitely if the tray failed to start.
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}
