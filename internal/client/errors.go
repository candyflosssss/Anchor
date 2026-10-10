package client

import "fmt"

// RequestError exposes only bounded classifications, never upstream text or credentials.
// Unknown means a mutating call may have reached the core and must not be replayed.
type RequestError struct {
	Status  int
	Code    string
	Unknown bool
}

func (e *RequestError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("management request failed: HTTP %d (%s); writes are not automatically retried", e.Status, e.Code)
	}
	return "management request failed (" + e.Code + "); no fallback target was used"
}

var ErrIdentity = fmt.Errorf("instance identity or API version mismatch; explicitly rebind the context")
