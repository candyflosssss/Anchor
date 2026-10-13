// Package modeldev provides reference metadata and text-token pricing from the
// public models.dev catalog. It never probes inference or writes configuration.
package modeldev

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalid     = errors.New("invalid model catalog request or data")
	ErrNotFound    = errors.New("model catalog entry not found")
	ErrUnavailable = errors.New("model catalog unavailable")
)

const MaxSafeInteger int64 = 9007199254740991

type ModelCatalogQuery struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
	Search     string `json:"search"`
	Limit      int    `json:"limit"`
	Refresh    bool   `json:"refresh"`
}

func (q ModelCatalogQuery) Validate() error {
	if !validText(q.ProviderID, 128, true) || !validText(q.ModelID, 256, true) || !validText(q.Search, 128, true) || q.Limit < 0 || q.Limit > 100 || (q.ProviderID == "" && (q.ModelID != "" || q.Search != "")) || (q.ModelID != "" && q.Search != "") {
		return ErrInvalid
	}
	return nil
}
func validText(v string, n int, empty bool) bool {
	return (empty || v != "") && len(v) <= n && utf8.ValidString(v) && strings.TrimSpace(v) == v && !strings.ContainsFunc(v, unicode.IsControl)
}

type CatalogProvider struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ModelCount int    `json:"model_count"`
}
type CatalogCapabilities struct {
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
	Vision           *bool    `json:"vision"`
	Attachment       *bool    `json:"attachment"`
	ToolCall         *bool    `json:"tool_call"`
	Reasoning        *bool    `json:"reasoning"`
	StructuredOutput *bool    `json:"structured_output"`
	Temperature      *bool    `json:"temperature"`
}
type CatalogLimits struct {
	Context *int64 `json:"context"`
	Input   *int64 `json:"input"`
	Output  *int64 `json:"output"`
}

// Null effort values are preserved; they are not the string "none". Budget -1
// is a provider-declared automatic budget, not a negative token limit.
type ReasoningOption struct {
	Type   string    `json:"type"`
	Values []*string `json:"values"`
	Min    *int64    `json:"min"`
	Max    *int64    `json:"max"`
}

// Decimal strings use USD per million tokens. Nil is unknown; "0" is free.
type CatalogRates struct {
	Input       *string `json:"input"`
	Output      *string `json:"output"`
	CacheRead   *string `json:"cache_read"`
	CacheWrite  *string `json:"cache_write"`
	Reasoning   *string `json:"reasoning"`
	InputAudio  *string `json:"input_audio"`
	OutputAudio *string `json:"output_audio"`
}
type CatalogTier struct {
	AboveTokens int64        `json:"above_tokens"`
	Rates       CatalogRates `json:"rates"`
}
type CatalogPricing struct {
	Currency string        `json:"currency"`
	Unit     string        `json:"unit"`
	Rates    CatalogRates  `json:"rates"`
	Tiers    []CatalogTier `json:"tiers"`
}
type CatalogModel struct {
	ProviderID       string              `json:"provider_id"`
	ID               string              `json:"id"`
	CanonicalModelID string              `json:"canonical_model_id"`
	Name             string              `json:"name"`
	SourceUpdatedAt  string              `json:"source_updated_at"`
	Capabilities     CatalogCapabilities `json:"capabilities"`
	Limits           CatalogLimits       `json:"limits"`
	ReasoningOptions []ReasoningOption   `json:"reasoning_options"`
	Pricing          *CatalogPricing     `json:"pricing"`
}
type ModelCatalog struct {
	Source    string            `json:"source"`
	FetchedAt int64             `json:"fetched_at"`
	ExpiresAt int64             `json:"expires_at"`
	Version   string            `json:"version"`
	Stale     bool              `json:"stale"`
	Failure   string            `json:"failure"`
	Providers []CatalogProvider `json:"providers"`
	Models    []CatalogModel    `json:"models"`
	Total     int               `json:"total"`
	Truncated bool              `json:"truncated"`
}

// Input includes cache reads/writes; output includes reasoning. Nil counts are
// unobserved, including cache counts. This estimates text tokens only.
type CatalogUsage struct {
	InputTokens      *int64 `json:"input_tokens"`
	OutputTokens     *int64 `json:"output_tokens"`
	CacheReadTokens  *int64 `json:"cache_read_tokens"`
	CacheWriteTokens *int64 `json:"cache_write_tokens"`
	ReasoningTokens  *int64 `json:"reasoning_tokens"`
}

func (u CatalogUsage) Validate() error {
	for _, n := range []*int64{u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens, u.ReasoningTokens} {
		if n != nil && (*n < 0 || *n > MaxSafeInteger) {
			return ErrInvalid
		}
	}
	if u.InputTokens != nil {
		var read, write int64
		if u.CacheReadTokens != nil {
			read = *u.CacheReadTokens
		}
		if u.CacheWriteTokens != nil {
			write = *u.CacheWriteTokens
		}
		if read > *u.InputTokens || write > *u.InputTokens-read {
			return ErrInvalid
		}
	}
	if u.OutputTokens != nil && u.ReasoningTokens != nil && *u.ReasoningTokens > *u.OutputTokens {
		return ErrInvalid
	}
	return nil
}

type CatalogEstimateInput struct {
	Query ModelCatalogQuery `json:"query"`
	Usage CatalogUsage      `json:"usage"`
}

func (in CatalogEstimateInput) Validate() error {
	if in.Query.ModelID == "" || in.Query.Limit != 0 || in.Query.Validate() != nil {
		return ErrInvalid
	}
	return in.Usage.Validate()
}

type CatalogEstimate struct {
	Catalog       ModelCatalog `json:"catalog"`
	Usage         CatalogUsage `json:"usage"`
	Basis         string       `json:"basis"`
	Currency      string       `json:"currency"`
	USD           *string      `json:"usd"`
	InputUSD      *string      `json:"input_usd"`
	OutputUSD     *string      `json:"output_usd"`
	CacheReadUSD  *string      `json:"cache_read_usd"`
	CacheWriteUSD *string      `json:"cache_write_usd"`
	ReasoningUSD  *string      `json:"reasoning_usd"`
	AboveTokens   *int64       `json:"above_tokens"`
	Reason        string       `json:"reason"`
}
