//go:build !windows

package systemproxy

import (
	"context"
	"net/url"
)

// Non-Windows hosts use proxy environment variables.
func ReadSystem(context.Context, *url.URL) (*url.URL, error) { return nil, nil }
