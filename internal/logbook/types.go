// Package logbook stores bounded, body-free request and system diagnostics.
package logbook

import (
	"errors"
	"time"

	"github.com/candyflosssss/Anchor/internal/modeldev"
)

var ErrInvalid = errors.New("invalid log operation")
var ErrNotFound = errors.New("log not found")
var ErrUnavailable = errors.New("log storage unavailable")

// All optional measurements are pointers: an absent observation is not zero.
type Metrics struct {
	// Output metrics include text and tool arguments, but exclude reasoning tokens.
	OutputGenerationMS       *int64 `json:"output_generation_ms"`
	NonReasoningOutputTokens *int64 `json:"non_reasoning_output_tokens"`
	OutputIntervalComplete   bool   `json:"output_interval_complete"`
	TimeToFirstOutputMS      *int64 `json:"time_to_first_output_ms"`
	TimeToFirstTextMS        *int64 `json:"time_to_first_text_ms"`
	GenerationMS             *int64 `json:"generation_ms"`
	OutputTextTokens         *int64 `json:"output_text_tokens"`
	TextIntervalComplete     bool   `json:"text_interval_complete"`
	CacheReadTokens          *int64 `json:"cache_read_tokens"`
	ReasoningTokens          *int64 `json:"reasoning_tokens"`
}

// ReferenceCost captures the reference used by this request, never a bill.
// USD is absent unless every dispatched attempt has a complete estimate.
type ReferenceCost struct {
	Adjustment     *CostAdjustment          `json:"adjustment,omitempty"`
	USD            *string                  `json:"usd"`
	Currency       string                   `json:"currency"`
	Basis          string                   `json:"basis"`
	Reason         string                   `json:"reason"`
	Source         string                   `json:"source"`
	ProviderID     string                   `json:"provider_id"`
	ModelID        string                   `json:"model_id"`
	CatalogVersion string                   `json:"catalog_version"`
	FetchedAt      int64                    `json:"fetched_at"`
	Stale          bool                     `json:"stale"`
	Pricing        *modeldev.CatalogPricing `json:"pricing"`
}
type Attempt struct {
	CostAdjustment *CostAdjustment `json:"cost_adjustment,omitempty"`
	ReferenceUSD   *string         `json:"reference_usd,omitempty"`
	Number         int             `json:"number"`
	Provider       string          `json:"provider"`
	Key            string          `json:"key"`
	Model          string          `json:"model"`
	Protocol       string          `json:"protocol"`
	HTTP           *int            `json:"http"`
	ElapsedMS      int64           `json:"elapsed_ms"`
	Outcome        string          `json:"outcome"`
	Code           string          `json:"code"`
	Stage          string          `json:"stage"`
	Input          *int64          `json:"input"`
	Output         *int64          `json:"output"`
}

// KnownUsage is the subtotal of attempts with confirmed input and output.
// It never replaces the root totals when another attempt's usage is unknown.
type KnownUsage struct {
	Input    int64 `json:"input"`
	Output   int64 `json:"output"`
	Attempts int   `json:"attempts"`
}
type Request struct {
	KnownUsage    *KnownUsage    `json:"known_usage,omitempty"`
	ReferenceCost *ReferenceCost `json:"reference_cost,omitempty"`
	// Agent and Binding come from authenticated enrollment, never caller labels.
	Agent   string `json:"agent,omitempty"`
	Binding string `json:"binding,omitempty"`
	// Project and Session are optional log labels: explicit client tags, or Codex
	// native thread IDs and project metadata resolved through an authenticated binding.
	Project           string            `json:"project,omitempty"`
	Session           string            `json:"session,omitempty"`
	ID                string            `json:"id"`
	StartedAtMS       int64             `json:"started_at_ms"`
	Status            string            `json:"status"`
	Source            string            `json:"source"`
	UpstreamModel     string            `json:"upstream_model"`
	Model             string            `json:"model"`
	Protocol          string            `json:"protocol"`
	ElapsedMS         *int64            `json:"elapsed_ms"`
	HTTP              *int              `json:"http"`
	Provider          string            `json:"provider"`
	Key               string            `json:"key"`
	Revision          int64             `json:"revision"`
	Delivery          string            `json:"delivery"`
	Stage             string            `json:"stage"`
	Code              string            `json:"code"`
	Input             *int64            `json:"input"`
	Output            *int64            `json:"output"`
	ReasoningEffort   *string           `json:"reasoning_effort"`
	ReasoningObserved bool              `json:"reasoning_observed"`
	Stream            *bool             `json:"stream"`
	GenerationStatus  string            `json:"generation_status"`
	Metrics           Metrics           `json:"metrics"`
	Attempts          []Attempt         `json:"attempts"`
	AttemptCount      int               `json:"attempt_count"`
	AttemptsTruncated bool              `json:"attempts_truncated"`
	Evidence          map[string]string `json:"evidence"`
}
type Event struct {
	ID        string            `json:"id"`
	AtMS      int64             `json:"at_ms"`
	Level     string            `json:"level"`
	Source    string            `json:"source"`
	Event     string            `json:"event"`
	RequestID string            `json:"request_id"`
	Fields    map[string]string `json:"fields"`
}
type Query struct {
	ErrorsOnly bool `json:"errors_only,omitempty"`
	// nil means all; a pointer to an empty string selects unattributed records.
	Agent    *string `json:"agent,omitempty"`
	Binding  *string `json:"binding,omitempty"`
	Project  *string `json:"project,omitempty"`
	Session  *string `json:"session,omitempty"`
	Kind     string  `json:"kind"`
	Limit    int     `json:"limit"`
	Cursor   int64   `json:"cursor"`
	Until    int64   `json:"until"`
	SinceMS  int64   `json:"since_ms"`
	BeforeMS int64   `json:"before_ms"`
	Status   string  `json:"status"`
	Source   string  `json:"source"`
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	Key      string  `json:"key"`
	HTTP     *int    `json:"http"`
	Code     string  `json:"code"`
	Stage    string  `json:"stage"`
	Level    string  `json:"level"`
	Search   string  `json:"search"`
}
type Page struct {
	Requests   []Request `json:"requests"`
	Events     []Event   `json:"events"`
	NextCursor int64     `json:"next_cursor"`
	Latest     int64     `json:"latest"`
	HasMore    bool      `json:"has_more"`
	Health     Health    `json:"health"`
}
type Settings struct {
	Days         int   `json:"days"`
	MaxRequests  int   `json:"max_requests"`
	RequestBytes int64 `json:"request_bytes"`
	SystemBytes  int64 `json:"system_bytes"`
	SystemDays   int   `json:"system_days"`
}

func DefaultSettings() Settings {
	return Settings{Days: 7, MaxRequests: 50000, RequestBytes: 256 << 20, SystemBytes: 32 << 20, SystemDays: 7}
}
func (s Settings) Validate() error {
	if s.Days < 1 || s.Days > 90 || s.SystemDays < 1 || s.SystemDays > 90 || s.MaxRequests < 100 || s.MaxRequests > 200000 || s.RequestBytes < 1<<20 || s.RequestBytes > 512<<20 || s.SystemBytes < 1<<20 || s.SystemBytes > 64<<20 {
		return ErrInvalid
	}
	return nil
}

type Health struct {
	Dropped       int64  `json:"dropped"`
	WriteFailures int64  `json:"write_failures"`
	QueueDepth    int    `json:"queue_depth"`
	RequestCount  int64  `json:"request_count"`
	RequestBytes  int64  `json:"request_bytes"`
	EventCount    int64  `json:"event_count"`
	EventBytes    int64  `json:"event_bytes"`
	Error         string `json:"error"`
}

func (q *Query) Validate() error {
	if q.Kind == "" {
		q.Kind = "requests"
	}
	if q.Kind != "requests" && q.Kind != "system" {
		return ErrInvalid
	}
	if q.ErrorsOnly && q.Kind != "requests" {
		return ErrInvalid
	}
	if q.Limit == 0 {
		q.Limit = 50
	}
	if q.Limit < 1 || q.Limit > 200 || q.Cursor < 0 || q.Until < 0 || (q.HTTP != nil && (*q.HTTP < 100 || *q.HTTP > 599)) {
		return ErrInvalid
	}
	if q.SinceMS == 0 {
		q.SinceMS = time.Now().Add(-time.Hour).UnixMilli()
	}
	if q.SinceMS < 0 || q.BeforeMS < 0 || (q.BeforeMS != 0 && q.BeforeMS < q.SinceMS) {
		return ErrInvalid
	}
	for _, value := range []*string{q.Agent, q.Binding, q.Project, q.Session} {
		if value != nil && (q.Kind != "requests" || len(*value) > 512) {
			return ErrInvalid
		}
	}
	for _, s := range []string{q.Status, q.Source, q.Provider, q.Model, q.Key, q.Code, q.Stage, q.Level, q.Search} {
		if len(s) > 512 {
			return ErrInvalid
		}
	}
	return nil
}
