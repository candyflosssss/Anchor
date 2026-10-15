package desktop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// ConfigureInputMethod runs before GTK initialization, only in the GUI host.
// GNOME Wayland's default GTK Wayland context cannot reach a standalone Fcitx5
// daemon. Prefer its installed GTK bridge in that case, without overriding an
// explicitly selected input method or changing the user's desktop configuration.
func ConfigureInputMethod() {
	configureInputMethod(os.Getenv, os.Setenv, fcitxGTKAvailable)
}

func configureInputMethod(getenv func(string) string, setenv func(string, string) error, available func() bool) {
	if getenv("GTK_IM_MODULE") != "" || getenv("WAYLAND_DISPLAY") == "" {
		return
	}
	gnome := false
	for _, desktop := range strings.Split(getenv("XDG_CURRENT_DESKTOP"), ":") {
		if strings.EqualFold(desktop, "GNOME") {
			gnome = true
		}
	}
	if gnome && available() {
		_ = setenv("GTK_IM_MODULE", "fcitx")
	}
}

func fcitxGTKAvailable() bool {
	// Do not select a missing module, and never start an input-method daemon.
	found := false
	for _, pattern := range []string{
		"/usr/lib/*/gtk-3.0/*/immodules/im-fcitx5.so",
		"/usr/lib/gtk-3.0/*/immodules/im-fcitx5.so",
		"/usr/lib64/gtk-3.0/*/immodules/im-fcitx5.so",
	} {
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if address == "" {
		return false
	}
	conn, err := dbus.Connect(address, dbus.WithContext(ctx))
	if err != nil {
		return false
	}
	defer conn.Close()
	var active bool
	err = conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, "org.fcitx.Fcitx5").Store(&active)
	return err == nil && active
}
