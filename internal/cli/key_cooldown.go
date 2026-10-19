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
	enabled := f.Bool("enabled", true, "enable automatic key downweighting (default true)")
	threshold := f.Int("failure-threshold", 3, "consecutive failures, 1..100 (default 3)")
	seconds := f.Int("cooldown-seconds", 30, "failure downweight seconds, 1..3600 (default 30)")
	rate := f.Int("rate-limit-seconds", 30, "429 fallback seconds, 1..3600 (default 30); Retry-After takes priority")
	weight := f.Int("weight-percent", 10, "retained scheduling weight, 1..100 percent (default 10)")
	revision := f.Int64("revision", 0, "expected settings revision (required when saving)")
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	visited := map[string]bool{}
	f.Visit(func(v *flag.Flag) { visited[v.Name] = true })
	writing := visited["weight-percent"] || visited["enabled"] || visited["failure-threshold"] || visited["cooldown-seconds"] || visited["rate-limit-seconds"]
	if !writing && visited["revision"] {
		return errors.New("--revision requires a settings change")
	}
	if writing {
		if err := (state.SaveKeyCooldownSettings{WeightPercent: weight, Enabled: enabled, FailureThreshold: threshold, CooldownSeconds: seconds, RateLimitSeconds: rate, ExpectedRevision: *revision}).Validate(); err != nil {
			return errors.New("invalid downweight settings; --revision is required")
		}
	}
	current, err := c.KeyCooldownSettings(ctx)
	if err != nil {
		return err
	}
	if !writing {
		return a.output(current)
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
	out, err := c.SaveKeyCooldownSettings(ctx, state.SaveKeyCooldownSettings{WeightPercent: weight, Enabled: enabled, FailureThreshold: threshold, CooldownSeconds: seconds, RateLimitSeconds: rate, ExpectedRevision: *revision})
	if err != nil {
		return err
	}
	return a.output(out)
}
