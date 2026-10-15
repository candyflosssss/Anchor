package core

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (s *Server) importPrice(ctx context.Context, alias, providerID, model string) (*catalog.Price, error) {
	if !state.ValidModelID(alias) || providerID == "" || model == "" {
		return nil, state.ErrInvalid
	}
	result, err := s.modelCatalog.Query(ctx, modeldev.ModelCatalogQuery{ProviderID: providerID, ModelID: model})
	if err != nil {
		if errors.Is(err, modeldev.ErrInvalid) {
			return nil, state.ErrInvalid
		}
		if errors.Is(err, modeldev.ErrNotFound) {
			return nil, state.ErrNotFound
		}
		return nil, errors.New("model reference catalog unavailable")
	}
	// The legacy price form cannot display catalog staleness or price tiers.
	if result.Stale {
		return nil, errors.New("model reference catalog refresh failed")
	}
	return referencePrice(result.Models[0], alias, result.FetchedAt)
}
func referencePrice(model modeldev.CatalogModel, alias string, fetchedAt int64) (*catalog.Price, error) {
	if model.Pricing == nil || model.Pricing.Rates.Input == nil || model.Pricing.Rates.Output == nil {
		return nil, state.ErrNotFound
	}
	r := model.Pricing.Rates
	value := func(v *string) *float64 {
		if v == nil {
			return nil
		}
		n, _ := strconv.ParseFloat(*v, 64)
		return &n
	}
	return &catalog.Price{Model: alias, InputUSD: *value(r.Input), OutputUSD: *value(r.Output), CacheReadUSD: value(r.CacheRead), CacheWriteUSD: value(r.CacheWrite), Source: "models.dev/" + model.ProviderID + "/" + model.ID, UpdatedAt: fetchedAt}, nil
}
func parsePrice(raw []byte, alias, providerID, model string) (*catalog.Price, error) {
	entry, err := modeldev.ParseModel(raw, providerID, model)
	if err != nil {
		if errors.Is(err, modeldev.ErrNotFound) {
			return nil, state.ErrNotFound
		}
		return nil, state.ErrInvalid
	}
	return referencePrice(entry, alias, time.Now().Unix())
}
