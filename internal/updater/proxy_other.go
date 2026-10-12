//go:build !windows

package updater

import (
	"context"
	"net/url"
)

// Non-Windows hosts use proxy environment variables.
func readSystemProxy(context.Context, *url.URL) (*url.URL, error) { return nil, nil }
