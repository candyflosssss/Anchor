package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/candyflosssss/Anchor/internal/catalog"
)

func (c *Client) ProviderChat(ctx context.Context, in catalog.ChatRequest) (catalog.ProbeProgress, error) {
	var out catalog.ProbeProgress
	err := c.call(ctx, http.MethodPost, "/control/v1/providers/"+url.PathEscape(in.ProviderID)+"/chat", in, &out)
	return out, err
}
