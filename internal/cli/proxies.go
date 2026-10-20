package cli

import (
	"context"
	"errors"
	"io"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (a *App) proxies(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 {
		return errors.New("proxy requires list, put, delete or bind")
	}
	if args[0] == "list" {
		if len(args) != 1 {
			return errors.New("proxy list takes no arguments")
		}
		v, e := c.ProxyWorkspace(ctx)
		if e != nil {
			return e
		}
		return a.output(v)
	}
	if len(args) < 2 {
		return errors.New("proxy operation requires an ID")
	}
	f := a.flags("proxy " + args[0])
	rev := f.Int64("revision", 0, "expected proxy workspace revision (required)")
	approved := f.Bool("approve", false, "approve configuration change (required)")
	name := f.String("name", "", "display name (put)")
	kind := f.String("kind", "", "http, socks5 or mihomo (put)")
	input := f.Bool("secret-stdin", false, "read proxy URL or Clash YAML from stdin; omit to retain existing secret")
	node := f.String("node", "", "node name when importing a multi-node Clash YAML")
	key := f.String("key", "", "Key ID; omitted for supplier default (bind)")
	mode := f.String("mode", "", "direct (no application proxy), system (require OS/environment proxy), proxy, or inherit for a Key (bind)")
	profile := f.String("proxy", "", "configured proxy ID for mode proxy")
	if e := parse(f, args[2:]); e != nil {
		return e
	}
	if *rev < 1 || !*approved {
		return errors.New("--revision and --approve are required")
	}
	in := state.ProxyAction{Action: args[0], ExpectedRevision: *rev, Approved: *approved}
	switch args[0] {
	case "put":
		in.Profile = state.ProxyProfile{ID: args[1], Name: *name, Kind: *kind}
		in.NodeName = *node
		if *input {
			b, e := io.ReadAll(io.LimitReader(a.In, (64<<10)+1))
			if e != nil {
				return e
			}
			if len(b) > 64<<10 {
				return errors.New("proxy configuration exceeds 64 KiB")
			}
			s := string(b)
			in.Secret = &s
		}
	case "delete":
		in.Profile.ID = args[1]
	case "bind":
		in.Binding = state.ProxyBinding{ProviderID: args[1], KeyID: *key, Mode: *mode, ProxyID: *profile}
	default:
		return errors.New("unknown proxy operation")
	}
	v, e := c.ProxyAction(ctx, in)
	if e != nil {
		return e
	}
	return a.output(v)
}
