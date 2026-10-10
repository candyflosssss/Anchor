//go:build desktop

package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"

	"github.com/candyflosssss/Anchor/desktop/branding"
	"github.com/candyflosssss/Anchor/desktop/frontend"
	"github.com/candyflosssss/Anchor/internal/cli"
	"github.com/candyflosssss/Anchor/internal/desktop"
	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

func main() { os.Exit(run()) }
func run() int {
	defaults, err := state.DefaultDir()
	if err != nil {
		return 1
	}
	flags := flag.NewFlagSet("anchor-desktop", flag.ContinueOnError)
	dir := flags.String("data-dir", defaults, "local instance directory")
	core := flags.Bool("core", false, "internal detached core mode")
	if flags.Parse(os.Args[1:]) != nil || flags.NArg() != 0 {
		return 2
	}
	if *core {
		ctx, cancel := signal.NotifyContext(context.Background(), shutdownSignals()...)
		defer cancel()
		return cli.Main(ctx, []string{"--data-dir", *dir, "serve"})
	}
	executable, err := os.Executable()
	if err != nil {
		return 1
	}
	session := desktop.NewSession(desktop.Options{Dir: *dir, Executable: executable})
	defer session.Close()
	assets, err := fs.Sub(frontend.Assets, "dist")
	if err != nil {
		return 1
	}
	err = wails.Run(&options.App{Title: "Anchor", Width: 1200, Height: 800, MinWidth: 720, MinHeight: 540,
		Frameless:   true,
		AssetServer: &assetserver.Options{Assets: assets}, Bind: []interface{}{desktop.NewBridge(session)},
		OnShutdown: func(context.Context) { session.Close() },
		Linux:      &linux.Options{Icon: branding.Icon, ProgramName: "anchor-desktop", WebviewGpuPolicy: linux.WebviewGpuPolicyNever},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Anchor desktop failed to start; check the platform runtime.")
		return 1
	}
	return 0
}
