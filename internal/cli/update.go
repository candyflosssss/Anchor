package cli

import (
	"context"
	"errors"
	"flag"
	"os"
	"time"

	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/updater"
)

func (a App) update(ctx context.Context, dir string, args []string) error {
	if len(args) == 0 {
		return errors.New("update requires config, configure, check, download or apply")
	}
	cfg, err := updater.ReadConfig(dir)
	if err != nil {
		return err
	}
	switch args[0] {
	case "config":
		if len(args) != 1 {
			return errors.New("update config takes no arguments")
		}
		return a.output(cfg)
	case "configure":
		f := a.flags("update configure")
		channel := f.String("channel", cfg.Channel, "stable, alpha, beta, rc or dev")
		manifest := f.String("manifest", cfg.ManifestURL, "HTTPS manifest URL; empty uses GitHub Releases")
		defaults := f.Bool("default-source", false, "reset to GitHub Releases")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *defaults {
			provided := false
			f.Visit(func(v *flag.Flag) {
				if v.Name == "manifest" {
					provided = true
				}
			})
			if provided {
				return errors.New("--default-source conflicts with --manifest")
			}
			*manifest = ""
		}
		cfg = updater.Config{Channel: *channel, ManifestURL: *manifest}
		if err := updater.Configure(dir, cfg); err != nil {
			return err
		}
		return a.output(cfg)
	case "check", "download":
		f := a.flags("update " + args[0])
		channel := f.String("channel", cfg.Channel, "channel override for this operation")
		manifest := f.String("manifest", cfg.ManifestURL, "source override for this operation")
		product := f.String("product", "cli", "cli or desktop")
		current := f.String("current-version", "", "required for desktop; omit for cli")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if *product != "cli" && *product != "desktop" {
			return updater.ErrInvalid
		}
		version := core.Version
		if *product == "desktop" {
			if *current == "" {
				return errors.New("--product desktop requires --current-version")
			}
			version = *current
		} else if *current != "" {
			return errors.New("--current-version is only accepted for desktop")
		}
		service := updater.New(*product, version)
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		result, err := service.Check(ctx, updater.Config{Channel: *channel, ManifestURL: *manifest})
		if err != nil {
			return err
		}
		if args[0] == "check" {
			return a.output(result)
		}
		downloaded, err := service.Download(ctx, dir, result)
		if err != nil {
			return err
		}
		return a.output(downloaded)
	case "apply":
		f := a.flags("update apply")
		approve := f.Bool("approve", false, "replace this CLI executable; local core must be stopped")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if !*approve {
			return errors.New("update apply requires --approve; replaces local CLI and retains .previous backup")
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		backup, err := updater.New("cli", core.Version).ApplyBinary(dir, executable)
		if err != nil {
			return err
		}
		return a.output(map[string]any{"installed": true, "backup": backup, "restart_required": true, "core_restarted": false})
	default:
		return errors.New("unknown update command")
	}
}
