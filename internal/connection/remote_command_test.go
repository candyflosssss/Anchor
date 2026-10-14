package connection

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func TestBootstrapDiscoveryWithoutLoginPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("remote POSIX shell fixture")
	}
	for _, tc := range []struct{ name, installed, requested, code string }{
		{"path", "path/anchor", "anchor", ""},
		{"user local", ".local/bin/anchor", "anchor", ""},
		{"user bin", "bin/anchor", "anchor", ""},
		{"explicit home", ".local/bin/anchor", "~/.local/bin/anchor", ""},
		{"explicit absolute", "custom/anchor", "absolute", ""},
		{"explicit missing never falls back", ".local/bin/anchor", "~/missing/anchor", "remote_anchor_missing"},
		{"unsupported", ".local/bin/anchor", "anchor", "remote_anchor_unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Spaces and shell metacharacters in HOME must remain literal path text.
			home := filepath.Join(t.TempDir(), "home with spaces;$(false)")
			pathDir := filepath.Join(home, "path")
			if err := os.MkdirAll(pathDir, 0700); err != nil {
				t.Fatal(err)
			}
			shell, err := exec.LookPath("sh")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(shell, filepath.Join(pathDir, "sh")); err != nil {
				t.Fatal(err)
			}
			installed := filepath.Join(home, tc.installed)
			if err := os.MkdirAll(filepath.Dir(installed), 0700); err != nil {
				t.Fatal(err)
			}
			script := "#!/bin/sh\nif [ \"$*\" = 'context bootstrap --help' ]; then exit 0; fi\nprintf '%s\\n' \"$@\"\n"
			if tc.code == "remote_anchor_unsupported" {
				script = "#!/bin/sh\nprintf '%s\\n' 'unknown context command synthetic-secret' >&2\nexit 1\n"
			}
			if err := os.WriteFile(installed, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			requested := tc.requested
			if requested == "absolute" {
				requested = installed
			}
			// The normal validator deliberately excludes spaces from explicit user input.
			// Internally quoted paths remain safe even when testing beyond that restriction.
			command := bootstrapCommand(state.Context{RemoteExecutable: requested, RemoteControlPort: 17861})
			cmd := exec.Command(shell, "-c", command)
			cmd.Env = []string{"HOME=" + home, "PATH=" + pathDir}
			var stderr strings.Builder
			cmd.Stderr = &stderr
			output, err := cmd.Output()
			if tc.code != "" {
				if err == nil {
					t.Fatal("expected failure")
				}
				var diagnostic *Error
				if !errors.As(remoteBootstrapError(stderr.String(), "remote_anchor"), &diagnostic) || diagnostic.Code != tc.code {
					t.Fatalf("diagnostic: %v", diagnostic)
				}
				if len(output) > 0 || strings.Contains(stderr.String(), "synthetic-secret") {
					t.Fatal("unexpected data disclosure")
				}
				return
			}
			if err != nil {
				t.Fatalf("discovery failed: %v, %s", err, stderr.String())
			}
			if string(output) != "context\nbootstrap\n--control-port\n17861\n--show-once\n" {
				t.Fatalf("incorrect arguments: %q", output)
			}
		})
	}
}

func TestRemoteBootstrapDiagnosticsNeverExposeStderr(t *testing.T) {
	for _, tc := range []struct{ stderr, code string }{
		{"anchor-bootstrap: remote_anchor_missing\nsynthetic-secret", "remote_anchor_missing"},
		{"anchor-bootstrap: remote_anchor_unsupported", "remote_anchor_unsupported"},
		{"anchor: connection failed (remote_control_port)\nsynthetic-secret", "remote_control_port"},
		{"anchor: connection failed (remote_core_not_ready)", "remote_core_not_ready"},
		{"random remote_anchor_missing text synthetic-secret", "remote_anchor"},
		{"Permission denied synthetic-secret", "remote_anchor"},
	} {
		var diagnostic *Error
		err := remoteBootstrapError(tc.stderr, "remote_anchor")
		if !errors.As(err, &diagnostic) || diagnostic.Code != tc.code || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatalf("unsafe diagnostic: %v", err)
		}
	}
}
