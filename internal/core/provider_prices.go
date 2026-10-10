package core

import (
	"context"
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
	"io"
	"net/http"
	"time"
)

func (s *Server) importPrice(ctx context.Context, alias, providerID, model string) (*catalog.Price, error) {
	if !state.ValidID(alias) || !state.ValidID(providerID) || model == "" || len(model) > 256 {
		return nil, state.ErrInvalid
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return nil, state.ErrConflict
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://models.dev/api.json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.upstream.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, state.ErrInvalid
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil || len(raw) > 16<<20 {
		return nil, state.ErrInvalid
	}
	return parsePrice(raw, alias, providerID, model)
}
func parsePrice(raw []byte, alias, providerID, model string) (*catalog.Price, error) {
	var data map[string]struct {
		Models map[string]struct {
			Cost *struct {
				Input      *float64 `json:"input"`
				Output     *float64 `json:"output"`
				CacheRead  *float64 `json:"cache_read"`
				CacheWrite *float64 `json:"cache_write"`
			} `json:"cost"`
		} `json:"models"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return nil, state.ErrInvalid
	}
	cost := data[providerID].Models[model].Cost
	if cost == nil || cost.Input == nil || cost.Output == nil {
		return nil, state.ErrNotFound
	}
	return &catalog.Price{Model: alias, InputUSD: *cost.Input, OutputUSD: *cost.Output, CacheReadUSD: cost.CacheRead, CacheWriteUSD: cost.CacheWrite, Source: "models.dev/" + providerID + "/" + model, UpdatedAt: time.Now().Unix()}, nil
}
