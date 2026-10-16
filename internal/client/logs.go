package client

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/logbook"
	"io"
	"net/http"
	"net/url"
)

func (c *Client) Logs(ctx context.Context, q logbook.Query) (logbook.Page, error) {
	var v logbook.Page
	err := c.call(ctx, "POST", "/control/v1/logs/query", q, &v)
	return v, err
}
func (c *Client) LogDetail(ctx context.Context, id string) (logbook.Request, error) {
	var v logbook.Request
	err := c.call(ctx, "GET", "/control/v1/logs/requests/"+url.PathEscape(id), nil, &v)
	return v, err
}
func (c *Client) LogSettings(ctx context.Context) (logbook.Settings, error) {
	var v logbook.Settings
	err := c.call(ctx, "GET", "/control/v1/logs/settings", nil, &v)
	return v, err
}
func (c *Client) ConfigureLogs(ctx context.Context, in logbook.Settings) (logbook.Settings, error) {
	var v logbook.Settings
	err := c.call(ctx, "PUT", "/control/v1/logs/settings", in, &v)
	return v, err
}

// Export streams to the caller's writer rather than buffering a history in memory.
func (c *Client) ExportLogs(ctx context.Context, q logbook.Query, out io.Writer) error {
	raw, err := json.Marshal(q)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/control/v1/logs/export", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Anchor-Instance", c.instance)
	req.Header.Set("Content-Type", "application/json")
	cl := *c.http
	cl.Timeout = 0
	resp, err := cl.Do(req)
	if err != nil {
		return &RequestError{Code: "offline"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &RequestError{Status: resp.StatusCode, Code: "log_export_failed"}
	}
	_, err = io.Copy(out, resp.Body)
	return err
}

func (c *Client) LogStats(ctx context.Context, q logbook.StatsQuery) (logbook.Stats, error) {
	var v logbook.Stats
	err := c.call(ctx, "POST", "/control/v1/logs/stats", q, &v)
	return v, err
}

func (c *Client) LogSuggestions(ctx context.Context, q logbook.SuggestionsQuery) (logbook.Suggestions, error) {
	var v logbook.Suggestions
	err := c.call(ctx, "POST", "/control/v1/logs/suggestions", q, &v)
	return v, err
}
