// Package catalog defines the public, read-only upstream model discovery contract.
package catalog

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxBytes = 512 << 10
const MaxModels = 1000

// DiscoverProviderModels pins the saved configuration used for this one request.
type DiscoverProviderModels struct {
	KeyID            string `json:"key_id"`
	ProviderID       string `json:"provider_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}

// ProviderModels describes a bounded directory lookup, not inference health.
// Failure is empty on success. Models is always an array, including on failure.
type ProviderModels struct {
	KeyID      string   `json:"key_id"`
	ProviderID string   `json:"provider_id"`
	Revision   int64    `json:"revision"`
	HTTPStatus int      `json:"http_status"`
	ElapsedMS  int64    `json:"elapsed_ms"`
	Models     []string `json:"models"`
	Failure    string   `json:"failure"`
}

// Page keeps the raw row count until the complete directory has been bounded.
// CursorParameter is fixed by the response format, never an upstream URL.
type Page struct {
	Models          []string
	Format          string
	CursorParameter string
	NextCursor      string
}

// Parse accepts a complete, single-page directory. Paginated callers use
// ParsePage so an unfinished directory cannot be mistaken for a complete list.
func Parse(raw []byte, secret string) ([]string, error) {
	page, err := ParsePage(raw, secret)
	if err != nil || page.NextCursor != "" {
		return nil, errors.New("invalid catalog")
	}
	slices.Sort(page.Models)
	return slices.Compact(page.Models), nil
}

// ParsePage accepts OpenAI/Anthropic data[].id or Gemini models[].name.
// Metadata is discarded; malformed rows or pagination fail the entire lookup.
func ParsePage(raw []byte, secret string) (Page, error) {
	invalid := errors.New("invalid catalog")
	var body struct {
		Data          json.RawMessage `json:"data"`
		Models        json.RawMessage `json:"models"`
		HasMore       *bool           `json:"has_more"`
		LastID        string          `json:"last_id"`
		NextPageToken string          `json:"nextPageToken"`
	}
	if len(raw) > MaxBytes || !utf8.Valid(raw) || json.Unmarshal(raw, &body) != nil {
		return Page{}, invalid
	}
	page := Page{Models: []string{}}
	var rows []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	var entries json.RawMessage
	switch {
	case len(body.Data) > 0 && len(body.Models) == 0:
		page.Format = "data"
		entries = body.Data
		if body.NextPageToken != "" {
			return Page{}, invalid
		}
		if body.HasMore != nil && *body.HasMore {
			page.CursorParameter, page.NextCursor = "after_id", body.LastID
			if !validValue(body.LastID, secret, 256) {
				return Page{}, invalid
			}
		}
	case len(body.Models) > 0 && len(body.Data) == 0:
		page.Format = "gemini"
		entries = body.Models
		if body.HasMore != nil || body.LastID != "" {
			return Page{}, invalid
		}
		if body.NextPageToken != "" {
			page.CursorParameter, page.NextCursor = "pageToken", body.NextPageToken
			if !validValue(body.NextPageToken, secret, 2048) {
				return Page{}, invalid
			}
		}
	default:
		return Page{}, invalid
	}
	if len(entries) == 0 || entries[0] != '[' || json.Unmarshal(entries, &rows) != nil || len(rows) > MaxModels {
		return Page{}, invalid
	}
	for _, row := range rows {
		id := row.ID
		if page.Format == "gemini" {
			// Some Gemini-compatible gateways return the bare model ID.
			id = strings.TrimPrefix(row.Name, "models/")
		}
		if !validValue(id, secret, 256) {
			return Page{}, invalid
		}
		page.Models = append(page.Models, id)
	}
	if page.NextCursor != "" && (len(rows) == 0 || (page.CursorParameter == "after_id" && body.LastID != rows[len(rows)-1].ID)) {
		return Page{}, invalid
	}
	return page, nil
}

func validValue(value, secret string, maxBytes int) bool {
	return value != "" && len(value) <= maxBytes && utf8.ValidString(value) && strings.TrimSpace(value) == value &&
		!strings.ContainsFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) &&
		(secret == "" || !strings.Contains(value, secret))
}
