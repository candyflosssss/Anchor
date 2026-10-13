package client

import (
	"context"
	"errors"
	"time"

	"github.com/candyflosssss/Anchor/internal/modeldev"
)

func (c *Client) ModelCatalog(ctx context.Context, in modeldev.ModelCatalogQuery) (modeldev.ModelCatalog, error) {
	if in.Validate() != nil {
		return modeldev.ModelCatalog{}, modeldev.ErrInvalid
	}
	var out modeldev.ModelCatalog
	err := c.catalogCall(ctx, "/control/v1/model-catalog/query", in, &out)
	return out, err
}
func (c *Client) EstimateModelCost(ctx context.Context, in modeldev.CatalogEstimateInput) (modeldev.CatalogEstimate, error) {
	if in.Validate() != nil {
		return modeldev.CatalogEstimate{}, modeldev.ErrInvalid
	}
	var out modeldev.CatalogEstimate
	err := c.catalogCall(ctx, "/control/v1/model-catalog/estimate", in, &out)
	return out, err
}
func catalogReadError(err error) error {
	var requestError *RequestError
	if errors.As(err, &requestError) {
		copy := *requestError
		copy.Unknown = false
		return &copy
	}
	return err
}

// Catalog reads may include a bounded cold download; other operations keep
// their existing shorter client timeout. Copies share only the HTTP transport.
func (c *Client) catalogCall(ctx context.Context, path string, in, out any) error {
	httpCopy := *c.http
	httpCopy.Timeout = 45 * time.Second
	copy := *c
	copy.http = &httpCopy
	return catalogReadError(copy.call(ctx, "POST", path, in, out))
}
