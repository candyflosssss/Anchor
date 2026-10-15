package desktop

import "testing"

func TestConfigureInputMethod(t *testing.T) {
	for _, tt := range []struct {
		name, module, wayland, desktop string
		available                      bool
		want                           string
	}{
		{"gnome fcitx", "", "wayland-0", "ubuntu:GNOME", true, "fcitx"},
		{"case insensitive", "", "wayland-0", "gnome", true, "fcitx"},
		{"respect ibus", "ibus", "wayland-0", "GNOME", true, "ibus"},
		{"respect disabled", "simple", "wayland-0", "GNOME", true, "simple"},
		{"respect wayland", "wayland", "wayland-0", "GNOME", true, "wayland"},
		{"no daemon or module", "", "wayland-0", "GNOME", false, ""},
		{"kde owns wayland input", "", "wayland-0", "KDE", true, ""},
		{"x11 unchanged", "", "", "GNOME", true, ""},
		{"no desktop", "", "wayland-0", "", true, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{"GTK_IM_MODULE": tt.module, "WAYLAND_DISPLAY": tt.wayland, "XDG_CURRENT_DESKTOP": tt.desktop}
			probes := 0
			configureInputMethod(func(k string) string { return env[k] }, func(k, v string) error { env[k] = v; return nil }, func() bool { probes++; return tt.available })
			if env["GTK_IM_MODULE"] != tt.want {
				t.Fatalf("got %q, want %q", env["GTK_IM_MODULE"], tt.want)
			}
			if (tt.module != "" || tt.wayland == "" || tt.desktop == "KDE" || tt.desktop == "") && probes != 0 {
				t.Fatal("unnecessary input-method probe")
			}
		})
	}
}
