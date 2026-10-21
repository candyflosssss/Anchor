package client

import (
	"context"
	"github.com/candyflosssss/Anchor/internal/ccswitch"
)

func (c *Client) ImportCCSwitch(ctx context.Context, in ccswitch.Request) (ccswitch.Report, error) {
	var out ccswitch.Report
	err := c.call(ctx, "POST", "/control/v1/import/cc-switch", in, &out)
	if !in.Approved {
		if request, ok := err.(*RequestError); ok {
			copy := *request
			copy.Unknown = false
			err = &copy
		}
	}
	return out, err
}
