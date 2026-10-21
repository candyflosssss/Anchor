package cli

import (
	"context"
	"errors"
	"flag"
	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (a App) keyCooldownSettings(ctx context.Context, c *client.Client, args []string) error {
	if len(args) == 0 || args[0] != "key-cooldown" {
		return errors.New("settings requires key-cooldown")
	}
	f := a.flags("settings key-cooldown")
	enabled := f.Bool("enabled", true, "enable automatic key cooldown (default true)")
	threshold := f.Int("failure-threshold", 100, "error score threshold, 1..100 (default 100)")
	window := f.Int("error-window-seconds", 300, "error score lifetime, 1..3600 (default 300); transient errors expire within 60 seconds")
	seconds := f.Int("cooldown-seconds", 300, "key cooldown seconds, 1..3600 (default 300)")
	rate := f.Int("rate-limit-seconds", 300, "429 fallback seconds, 1..3600 (default 300); cooldown uses the longer delay")
	weight := f.Int("weight-percent", 10, "deprecated compatibility field, 1..100 (ignored by cooldown)")
	revision := f.Int64("revision", 0, "expected settings revision (required when saving)")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	visited := map[string]bool{}
	f.Visit(func(v *flag.Flag) { visited[v.Name] = true })
	writing := visited["error-window-seconds"] || visited["weight-percent"] || visited["enabled"] || visited["failure-threshold"] || visited["cooldown-seconds"] || visited["rate-limit-seconds"]
	if !writing && visited["revision"] {
		return errors.New("--revision requires a settings change")
	}
	if writing {
		if err := (state.SaveKeyCooldownSettings{ErrorWindowSeconds: window, WeightPercent: weight, Enabled: enabled, FailureThreshold: threshold, CooldownSeconds: seconds, RateLimitSeconds: rate, ExpectedRevision: *revision}).Validate(); err != nil {
			return errors.New("invalid cooldown settings; --revision is required")
		}
	}
	current, err := c.KeyCooldownSettings(ctx)
	if err != nil {
		return err
	}
	if !writing {
		return a.output(current)
	}
	if !visited["error-window-seconds"] {
		*window = current.ErrorWindowSeconds
	}
	if !visited["weight-percent"] {
		*weight = current.WeightPercent
	}
	if !visited["enabled"] {
		*enabled = current.Enabled
	}
	if !visited["failure-threshold"] {
		*threshold = current.FailureThreshold
	}
	if !visited["cooldown-seconds"] {
		*seconds = current.CooldownSeconds
	}
	if !visited["rate-limit-seconds"] {
		*rate = current.RateLimitSeconds
	}
	out, err := c.SaveKeyCooldownSettings(ctx, state.SaveKeyCooldownSettings{ErrorWindowSeconds: window, WeightPercent: weight, Enabled: enabled, FailureThreshold: threshold, CooldownSeconds: seconds, RateLimitSeconds: rate, ExpectedRevision: *revision})
	if err != nil {
		return err
	}
	return a.output(out)
}
