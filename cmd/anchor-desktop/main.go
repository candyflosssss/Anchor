//go:build desktop

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/candyflosssss/Anchor/desktop/branding"
	"github.com/candyflosssss/Anchor/desktop/frontend"
	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/desktop"
	"github.com/candyflosssss/Anchor/internal/desktoptray"
	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/candyflosssss/Anchor/internal/updater"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func main() { os.Exit(run()) }
func run() int {
	// The managed adapter's token helper uses the same exact invocation as CLI.
	// Dispatch only this internal command before GUI flags/window initialization.
	if desktop.IsAgentTokenInvocation(os.Args[1:]) {
		return cli.Main(context.Background(), os.Args[1:])
	}

	defaults, err := state.DefaultDir()
	if err != nil {
		return 1
	}
	flags := flag.NewFlagSet("anchor-desktop", flag.ContinueOnError)
	dir := flags.String("data-dir", defaults, "local instance directory (does not allow multiple GUI windows)")
	finish := flags.Int("finish-update", 0, "internal installer handoff mode")
	installVersion := flags.String("install-version", "", "internal confirmed installer version")
	installHash := flags.String("install-sha256", "", "internal confirmed installer checksum")
	manifest := flags.String("update-manifest", "", "host-only update manifest URL (first GUI launch only)")
	channel := flags.String("update-channel", "", "host-only update channel (first GUI launch only)")
	coreMode := flags.Bool("core", false, "internal detached core mode")
	if flags.Parse(os.Args[1:]) != nil || flags.NArg() != 0 {
		return 2
	}
	if *finish != 0 {
		if *coreMode {
			return 2
		}
		if updater.RunHelper(*dir, core.Version, *finish, updater.InstallApproval{Version: *installVersion, SHA256: *installHash}) != nil {
			return 1
		}
		return 0
	}
	if *installVersion != "" || *installHash != "" {
		return 2
	}
	if *coreMode {
		ctx, cancel := signal.NotifyContext(context.Background(), shutdownSignals()...)
		defer cancel()
		return cli.Main(ctx, []string{"--data-dir", *dir, "serve"})
	}
	var window desktop.WindowLifecycle
	activationContext, cancelActivation := context.WithTimeout(context.Background(), 5*time.Second)
	host, err := desktop.AcquireHost(activationContext, defaults, window.Show)
	cancelActivation()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Anchor is already starting or its desktop could not be opened; try again shortly.")
		return 1
	}
	if host == nil {
		return 0
	}
	defer host.Close()
	var updateConfigProvided bool
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "update-manifest" || f.Name == "update-channel" {
			updateConfigProvided = true
		}
	})
	if updateConfigProvided {
		cfg, err := updater.ReadConfig(*dir)
		if err != nil {
			return 2
		}
		flags.Visit(func(f *flag.Flag) {
			if f.Name == "update-manifest" {
				cfg.ManifestURL = *manifest
			}
			if f.Name == "update-channel" {
				cfg.Channel = *channel
			}
		})
		if err := updater.Configure(*dir, cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Invalid update source/channel or unwritable local update directory.")
			return 2
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return 1
	}
	var hostContext context.Context
	var contextMu sync.RWMutex
	var tray desktoptray.Tray
	session := desktop.NewSession(desktop.Options{Dir: *dir, Executable: executable, SaveLogFile: func() (string, error) {
		contextMu.RLock()
		ctx := hostContext
		contextMu.RUnlock()
		if ctx == nil {
			return "", errors.New("window unavailable")
		}
		return wailsruntime.SaveFileDialog(ctx, wailsruntime.SaveDialogOptions{Title: "导出日志", DefaultFilename: "anchor-logs.jsonl", Filters: []wailsruntime.FileFilter{{DisplayName: "JSON Lines", Pattern: "*.jsonl"}}})
	}, Quit: func() {
		window.RequestExit()
		go func() {
			time.Sleep(250 * time.Millisecond)
			contextMu.RLock()
			ctx := hostContext
			contextMu.RUnlock()
			if ctx != nil {
				wailsruntime.Quit(ctx)
			}
		}()
	}})
	defer session.Close()
	defer tray.Stop()
	bridge := desktop.NewBridge(session)
	assets, err := fs.Sub(frontend.Assets, "dist")
	if err != nil {
		return 1
	}
	desktop.ConfigureInputMethod()
	err = wails.Run(&options.App{Title: "Anchor", Width: 1200, Height: 800, MinWidth: 720, MinHeight: 540,
		Frameless:   true,
		AssetServer: &assetserver.Options{Assets: assets}, Bind: []interface{}{bridge},
		OnStartup: func(ctx context.Context) {
			contextMu.Lock()
			hostContext = ctx
			contextMu.Unlock()
			tray.Start(func() { window.Show() }, func() { bridge.ExitDesktop() }, func() { exitWithLocalCore(ctx, &window, session) })
		},
		OnDomReady: func(ctx context.Context) {
			window.Ready(func() {
				if window.Exiting() {
					return
				}
				if wailsruntime.WindowIsMinimised(ctx) {
					wailsruntime.WindowUnminimise(ctx)
				}
				wailsruntime.WindowShow(ctx)
			})
		},
		OnShutdown: func(context.Context) { window.RequestExit(); tray.Stop(); session.Close() },
		OnBeforeClose: func(ctx context.Context) bool {
			// Hide only while the tray offers a way back. Explicit exit and update
			// installation must still terminate the host and its tray.
			return window.BeforeClose(tray.Available(), func() { wailsruntime.WindowHide(ctx) })
		},
		Linux: &linux.Options{Icon: branding.Icon, ProgramName: "anchor-desktop", WebviewGpuPolicy: linux.WebviewGpuPolicyNever},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Anchor desktop failed to start; check the platform runtime.")
		return 1
	}
	return 0
}
