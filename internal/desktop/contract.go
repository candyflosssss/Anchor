// Package desktop is the platform-independent, typed desktop boundary.
package desktop

const Contract = "anchor-desktop-v1"
const MaxSafeRevision int64 = 9007199254740991

// ProblemCodes is the sole source for generated TypeScript error literals.
var ProblemCodes = []string{"offline", "unauthorized", "read_error", "conflict", "unknown", "invalid", "startup_failed", "identity", "unsupported", "closed"}

type Problem struct {
	Code string `json:"code"`
}
type Provider struct {
	ID             string `json:"id"`
	BaseURL        string `json:"base_url"`
	Protocol       string `json:"protocol"`
	Enabled        bool   `json:"enabled"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxConcurrent  int    `json:"max_concurrent"`
	Revision       int64  `json:"revision"`
	HasSecret      bool   `json:"has_secret"`
}
type SaveProvider struct {
	Provider         Provider `json:"provider"`
	ExpectedRevision int64    `json:"expected_revision"`
	Secret           *string  `json:"secret"`
}
type SessionInfo struct {
	InstanceID string `json:"instance_id"`
	Phase      string `json:"phase"`
	Source     string `json:"source"`
	CanStop    bool   `json:"can_stop"`
}
type ConfirmStop struct {
	InstanceID string `json:"instance_id"`
	Approved   bool   `json:"approved"`
}
type StopInfo struct {
	Accepted bool `json:"accepted"`
}
type SessionResult struct {
	Contract string       `json:"contract"`
	Data     *SessionInfo `json:"data"`
	Problem  *Problem     `json:"problem"`
}
type ProvidersResult struct {
	Contract string     `json:"contract"`
	Data     []Provider `json:"data"`
	Problem  *Problem   `json:"problem"`
}
type SaveResult struct {
	Contract string    `json:"contract"`
	Data     *Provider `json:"data"`
	Problem  *Problem  `json:"problem"`
}
type StopResult struct {
	Contract string    `json:"contract"`
	Data     *StopInfo `json:"data"`
	Problem  *Problem  `json:"problem"`
}
