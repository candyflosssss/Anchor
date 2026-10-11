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

// ProviderModels describes a single directory request, not inference health.
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

// Parse accepts the OpenAI-style {data:[{id:...}]} directory, discards metadata,
// and fails closed rather than returning a silently truncated or partial list.
func Parse(raw []byte, secret string) ([]string, error) {
	var body struct {
		Data json.RawMessage `json:"data"`
	}
	if len(raw) > MaxBytes || !utf8.Valid(raw) || json.Unmarshal(raw, &body) != nil || len(body.Data) == 0 || body.Data[0] != '[' {
		return nil, errors.New("invalid catalog")
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body.Data, &rows) != nil || len(rows) > MaxModels {
		return nil, errors.New("invalid catalog")
	}
	models := make([]string, 0, len(rows))
	for _, row := range rows {
		id := row.ID
		if id == "" || len(id) > 256 || !utf8.ValidString(id) || strings.TrimSpace(id) != id || strings.ContainsFunc(id, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) || (secret != "" && strings.Contains(id, secret)) {
			return nil, errors.New("invalid catalog")
		}
		models = append(models, id)
	}
	slices.Sort(models)
	return slices.Compact(models), nil
}
