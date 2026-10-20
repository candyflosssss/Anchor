package modeldev

import "time"

// StartRefresh prefetches once, then keeps the shared catalog fresh without
// blocking core startup or inference. Only a running core starts this worker;
// constructing a client or an HTTP handler alone never starts network traffic.
func (c *Client) StartRefresh() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refreshDone != nil || c.ctx.Err() != nil {
		return
	}
	c.refreshDone = make(chan struct{})
	go c.maintainCatalog(c.refreshDone)
}

// RefreshSoon checks freshness after a committed mapping mutation. The public
// catalog contains all models, so fresh data needs no per-model download.
// A bounded mailbox and load's single flight coalesce bursts of edits.
func (c *Client) RefreshSoon() {
	select {
	case c.refreshWake <- struct{}{}:
	default:
	}
}

func (c *Client) maintainCatalog(done chan struct{}) {
	defer close(done)
	// load enforces the hourly TTL and a one-minute failure backoff. Checking
	// each minute also retries failed startup/refreshes without waiting an hour.
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		_, _, _ = c.load(c.ctx, false)
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
		case <-c.refreshWake:
		}
	}
}
