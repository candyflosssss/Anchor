//go:build desktop && linux

package desktoptray

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/candyflosssss/Anchor/desktop/branding"
	"github.com/godbus/dbus/v5"
)

func icon() []byte { return branding.Icon }
func platformSupported() bool {
	_, err := dbus.SessionBus()
	return err == nil
}

// A D-Bus connection alone is not proof that the desktop displays tray icons.
// Check that this process is registered with a live StatusNotifier host before
// hiding its only window. This also covers desktops without a tray extension.
func platformAvailable() bool {
	conn, err := dbus.SessionBus()
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	watcher := conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher")
	var host dbus.Variant
	if err := watcher.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0,
		"org.kde.StatusNotifierWatcher", "IsStatusNotifierHostRegistered").Store(&host); err != nil {
		return false
	}
	if active, ok := host.Value().(bool); !ok || !active {
		return false
	}
	var registered dbus.Variant
	if err := watcher.CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0,
		"org.kde.StatusNotifierWatcher", "RegisteredStatusNotifierItems").Store(&registered); err != nil {
		return false
	}
	items, ok := registered.Value().([]string)
	if !ok {
		return false
	}
	names := append(conn.Names(), fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid()))
	for _, name := range names {
		if slices.Contains(items, name+"/StatusNotifierItem") || slices.Contains(items, name) {
			return true
		}
	}
	return false
}
