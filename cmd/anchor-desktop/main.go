//go:build desktop

package main

import (
	"context"
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
	defaults, err := state.DefaultDir()
	if err != nil {
		return 1
	}
	flags := flag.NewFlagSet("anchor-desktop", flag.ContinueOnError)
	dir := flags.String("data-dir", defaults, "local instance directory")
	finish := flags.Int("finish-update", 0, "internal installer handoff mode")
	installVersion := flags.String("install-version", "", "internal confirmed installer version")
	installHash := flags.String("install-sha256", "", "internal confirmed installer checksum")
	manifest := flags.String("update-manifest", "", "host-only update manifest URL")
	channel := flags.String("update-channel", "", "host-only update channel")
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
	var window desktop.WindowLifecycle
	var tray desktoptray.Tray
	session := desktop.NewSession(desktop.Options{Dir: *dir, Executable: executable, Quit: func() {
		go func() {
			time.Sleep(250 * time.Millisecond)
			contextMu.RLock()
			ctx := hostContext
			contextMu.RUnlock()
			if ctx != nil {
				window.RequestExit()
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
	err = wails.Run(&options.App{Title: "Anchor", Width: 1200, Height: 800, MinWidth: 720, MinHeight: 540,
		Frameless:   true,
		AssetServer: &assetserver.Options{Assets: assets}, Bind: []interface{}{bridge},
		OnStartup: func(ctx context.Context) {
			contextMu.Lock()
			hostContext = ctx
			contextMu.Unlock()
			tray.Start(func() {
				if !window.Exiting() {
					if wailsruntime.WindowIsMinimised(ctx) {
						wailsruntime.WindowUnminimise(ctx)
					}
					wailsruntime.WindowShow(ctx)
				}
			}, func() { bridge.ExitDesktop() })
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
