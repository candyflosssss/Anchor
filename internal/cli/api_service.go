package cli

import (
	"context"
	"errors"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (a App) apiService(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 {
		return errors.New("api requires settings, configure, keys, issue or revoke")
	}
	switch args[0] {
	case "settings", "keys":
		if len(args) != 1 {
			return errors.New("unexpected arguments")
		}
		v, err := c.APIService(ctx)
		if err != nil {
			return err
		}
		if args[0] == "keys" {
			return a.output(v.Keys)
		}
		return a.output(v)
	case "configure":
		f := a.flags("api configure")
		enabled := f.Bool("enabled", false, "enable application listener")
		port := f.Int("port", 17862, "application port (1..65535)")
		lan := f.Bool("allow-lan", false, "allow private IPv4 LAN clients")
		rev := f.Int64("revision", 0, "current API settings revision (required)")
		approve := f.Bool("approve", false, "approve listener change, applied on core restart")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if !*approve {
			return errors.New("--approve is required")
		}
		v, err := c.SaveAPIService(ctx, state.SaveAPIServiceSettings{Enabled: enabled, Port: *port, AllowLAN: lan, ExpectedRevision: *rev})
		if err != nil {
			return err
		}
		return a.output(v)
	case "issue":
		f := a.flags("api issue")
		label := f.String("label", "", "application Key name (required)")
		ttl := f.Int64("ttl-seconds", 2592000, "validity in seconds; 0 never expires")
		show := f.Bool("show-once", false, "approve output of secret token once")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if !*show {
			return errors.New("--show-once is required")
		}
		v, err := c.IssueAPIKey(ctx, state.IssueAPIKey{Label: *label, TTLSeconds: *ttl})
		if err != nil {
			return err
		}
		return a.output(v)
	case "revoke":
		f := a.flags("api revoke")
		id := f.String("id", "", "gateway access ID (required)")
		approve := f.Bool("approve", false, "approve revoking this application Key")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if !*approve {
			return errors.New("--approve is required")
		}
		ok, err := c.RevokeAPIKey(ctx, *id)
		if err != nil {
			return err
		}
		return a.output(struct {
			Revoked bool `json:"revoked"`
		}{ok})
	}
	return errors.New("unknown api command")
}
