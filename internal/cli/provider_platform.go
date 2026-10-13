package cli

import (
	"context"
	"errors"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/providerapi"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (a App) providerPlatform(ctx context.Context, c *client.Client, args []string) error {
	f := a.flags("provider " + args[0])
	id := f.String("id", "", "saved provider ID")
	if args[0] == "platform" {
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		out, err := c.ProviderPlatform(ctx, *id)
		if err != nil {
			return err
		}
		return a.output(out)
	}
	rev := f.Int64("revision", 0, "expected provider revision (required)")
	if args[0] == "platform-put" {
		platform := f.String("platform", "auto", "auto (default), sub2api, newapi or oneapi")
		site := f.String("site-url", "", "dashboard deployment root; empty derives from request URL")
		userID := f.Int64("user-id", 0, "NewAPI user ID for deployments requiring New-Api-User")
		stdin := f.Bool("user-token-stdin", false, "read dashboard user token from stdin")
		clear := f.Bool("clear-user-token", false, "remove saved dashboard user authorization")
		if err := parse(f, args[1:]); err != nil {
			return err
		}
		if !state.ValidProviderID(*id) || *rev < 1 || *platform == "" || *stdin && *clear {
			return errors.New("platform-put requires --id and --revision; token input and clear are exclusive")
		}
		in := providerapi.Configure{ProviderID: *id, ExpectedRevision: *rev, Config: providerapi.Config{Platform: *platform, SiteURL: *site, UserID: *userID}}
		if *stdin {
			token, err := a.secret()
			if err != nil {
				return err
			}
			if token == "" {
				return errors.New("user token stdin is empty; use --clear-user-token to remove it")
			}
			in.UserToken = &token
		}
		if *clear {
			token := ""
			in.UserToken = &token
		}
		out, err := c.ConfigureProviderPlatform(ctx, in)
		if err != nil {
			return err
		}
		return a.output(out)
	}
	key := f.String("key", "", "local Key ID; default is enabled default key, then first enabled key")
	remote := f.Int64("remote-key-id", 0, "site numeric Key ID (OneAPI quota / effective rate)")
	scope := ""
	page, size := 0, 0
	if args[0] == "usage-records" {
		f.StringVar(&scope, "scope", "account", "account or key")
		f.IntVar(&page, "page", 1, "one-based page; NewAPI Key logs only support page 1")
		f.IntVar(&size, "page-size", 50, "page size 1..100; original OneAPI uses server page size")
	}
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	if args[0] == "usage-records" && (page < 1 || size < 1) {
		return errors.New("page and page-size must be positive")
	}
	out, err := c.QueryProvider(ctx, providerapi.Query{ProviderID: *id, ExpectedRevision: *rev, Operation: args[0], KeyID: *key, RemoteKeyID: *remote, Scope: scope, Page: page, PageSize: size})
	if err != nil {
		return err
	}
	// A classified upstream failure remains a valid result (like provider models),
	// so callers can distinguish missing authorization from a zero allowance.
	return a.output(out)
}
