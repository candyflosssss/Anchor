package cli

import (
	"context"
	"errors"

	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/modeldev"
)

func (a App) modelCatalog(ctx context.Context, cl *client.Client, args []string) error {
	if len(args) == 0 {
		return errors.New("catalog requires providers, models, model or estimate")
	}
	action := args[0]
	if action != "providers" && action != "models" && action != "model" && action != "estimate" {
		return errors.New("unknown catalog command")
	}
	f := a.flags("catalog " + action)
	refresh := f.Bool("refresh", false, "explicitly refresh the public models.dev catalog")
	q := modeldev.ModelCatalogQuery{}
	var provider, model, search *string
	var limit *int
	if action != "providers" {
		provider = f.String("provider", "", "exact models.dev provider ID")
	}
	if action == "models" {
		search = f.String("search", "", "case-insensitive model ID/name substring")
		limit = f.Int("limit", 50, "maximum returned models (1..100)")
	}
	if action == "model" || action == "estimate" {
		model = f.String("model", "", "exact models.dev model ID")
	}
	counts := map[string]*int64{}
	if action == "estimate" {
		for _, name := range []string{"input-tokens", "output-tokens", "cache-read-tokens", "cache-write-tokens", "reasoning-tokens"} {
			counts[name] = f.Int64(name, -1, "observed text token count; omit for unknown (totals include cache/reasoning)")
		}
	}
	if err := parse(f, args[1:]); err != nil {
		return err
	}
	q.Refresh = *refresh
	if provider != nil {
		q.ProviderID = *provider
		if q.ProviderID == "" {
			return errors.New("--provider is required")
		}
	}
	if model != nil {
		q.ModelID = *model
		if q.ModelID == "" {
			return errors.New("--model is required")
		}
	}
	if search != nil {
		q.Search = *search
		q.Limit = *limit
		if q.Limit < 1 {
			return errors.New("--limit must be 1..100")
		}
	}
	if q.Validate() != nil {
		return modeldev.ErrInvalid
	}
	if action != "estimate" {
		out, err := cl.ModelCatalog(ctx, q)
		if err != nil {
			return err
		}
		return a.output(out)
	}
	count := func(name string) (*int64, error) {
		v := *counts[name]
		if v < -1 {
			return nil, modeldev.ErrInvalid
		}
		if v == -1 {
			return nil, nil
		}
		return &v, nil
	}
	u := modeldev.CatalogUsage{}
	for _, pair := range []struct {
		name string
		dst  **int64
	}{{"input-tokens", &u.InputTokens}, {"output-tokens", &u.OutputTokens}, {"cache-read-tokens", &u.CacheReadTokens}, {"cache-write-tokens", &u.CacheWriteTokens}, {"reasoning-tokens", &u.ReasoningTokens}} {
		v, err := count(pair.name)
		if err != nil {
			return err
		}
		*pair.dst = v
	}
	out, err := cl.EstimateModelCost(ctx, modeldev.CatalogEstimateInput{Query: q, Usage: u})
	if err != nil {
		return err
	}
	return a.output(out)
}
