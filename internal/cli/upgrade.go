package cli

import (
	"context"
	"errors"
	"time"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/connection"
	"github.com/candyflosssss/Anchor/internal/core"
	"github.com/candyflosssss/Anchor/internal/hostipc"
	"github.com/candyflosssss/Anchor/internal/state"
	"github.com/candyflosssss/Anchor/internal/updateflow"
	"github.com/candyflosssss/Anchor/internal/updater"
)

func (a App) upgrade(ctx context.Context, dir, product string, args []string) error {
	cfg, err := updater.ReadConfig(dir)
	if err != nil {
		return err
	}
	if product == "desktop" {
		cfg, err = updater.ReadConfigForVersion(dir, core.Version)
		if err != nil {
			return err
		}
	}
	f := a.flags("update " + product)
	approve := f.Bool("approve", false, "prepare update then briefly restart the local core")
	channel := f.String("channel", cfg.Channel, "release channel")
	manifest := f.String("manifest", cfg.ManifestURL, "trusted HTTPS source")
	expected := f.String("expected-instance", "", "require this local instance identity")
	port := f.Int("control-port", 0, "require this control port with --expected-instance")
	version := f.String("expected-version", "", "require this exact checked release")
	hash := f.String("expected-sha256", "", "require this exact checked artifact")
	if err := parse(f, args); err != nil {
		return err
	}
	if !*approve {
		return errors.New("update requires --approve; service briefly restarts after preparation")
	}
	if (*version == "") != (*hash == "") {
		return errors.New("expected version and SHA256 must be provided together")
	}
	if _, err := updateflow.VerifyLocal(dir, *expected, *port); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	service := updater.New(product, core.Version)
	active, err := updater.ActiveRuntime(dir, product, core.Version)
	if err != nil {
		return err
	}
	if active.Version != "" {
		service.Version = active.Version
	}
	// Refuse to offer a release older than the authenticated running service.
	st, e := state.Open(dir, false)
	if e != nil {
		return e
	}
	settings, e := st.Settings()
	if e != nil {
		st.Close()
		return e
	}
	token, e := st.OwnerToken()
	st.Close()
	if e != nil {
		return e
	}
	cl, info, e := client.Connect(ctx, state.Context{Name: "update", Endpoint: "http://" + settings.ControlAddress, InstanceID: settings.InstanceID, CredentialRef: "local-owner"}, token)
	if e == nil {
		cl.Close()
		newer, e := updater.IsNewerVersion(info.Version, service.Version)
		if e != nil {
			return e
		}
		if newer {
			service.Version = info.Version
		}
	} else {
		var request *client.RequestError
		if !errors.As(e, &request) || request.Code != "offline" {
			return e
		}
	}
	c, err := service.Check(ctx, updater.Config{Channel: *channel, ManifestURL: *manifest})
	if err != nil {
		return err
	}
	if !c.Available {
		return a.output(map[string]any{"updated": false, "current_version": service.Version})
	}
	if *version != "" && (c.Version != *version || c.Artifact.SHA256 != *hash) {
		return updater.ErrInvalid
	}
	if product == "desktop" && c.Artifact.Kind != "runtime" {
		return errors.New("this release has no seamless runtime; use its native installer once")
	}
	d, err := service.Download(ctx, dir, c)
	if err != nil {
		return err
	}
	result, err := updateflow.Apply(ctx, dir, service, updater.InstallApproval{Version: d.Check.Version, SHA256: d.Check.Artifact.SHA256}, *expected, *port)
	if err != nil {
		return err
	}
	if product == "desktop" {
		if defaults, e := state.DefaultDir(); e == nil {
			if _, e = hostipc.Reload(ctx, defaults, dir); e != nil {
				return errors.New("new core is ready; desktop could not reopen automatically, close and reopen its window")
			}
		}
	}
	return a.output(result)
}

func (a App) remoteUpdate(ctx context.Context, dir, target string, args []string) error {
	if len(args) == 0 || (args[0] != "check" && args[0] != "upgrade") {
		return errors.New("SSH updates support update check or update upgrade --approve only")
	}
	f := a.flags("remote update " + args[0])
	approve := f.Bool("approve", false, "update only the selected SSH core")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if args[0] == "upgrade" && !*approve {
		return errors.New("remote update upgrade requires --approve")
	}
	st, err := state.Open(dir, false)
	if err != nil {
		return err
	}
	defer st.Close()
	profile, err := st.Context(target)
	if err != nil {
		return err
	}
	if profile.Transport != "ssh" {
		return errors.New("remote update requires an SSH context")
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	live, err := connection.OpenWithPassword(ctx, st, profile, a.sshPassword)
	if err != nil {
		return err
	}
	defer live.Close()
	c, err := live.CheckCoreUpdate(ctx)
	if err != nil {
		return err
	}
	if args[0] == "check" || !c.Available {
		return a.output(c)
	}
	result, err := live.UpgradeCore(ctx, c)
	if err != nil {
		return err
	}
	return a.output(result)
}
