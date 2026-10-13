package client

import (
	"context"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"net/http"
	"net/url"
)

func (c *Client) ProviderProbe(ctx context.Context, in catalog.ProbeRequest) (catalog.ProbeProgress, error) {
	var out catalog.ProbeProgress
	err := c.call(ctx, http.MethodPost, "/control/v1/providers/"+url.PathEscape(in.ProviderID)+"/probe", in, &out)
	return out, err
}
