// Package ccswitch reads CC Switch's provider inventory without changing it.
// Only the core uses this package; public reports never contain credentials.
package ccswitch

import "github.com/candyflosssss/Anchor/internal/state"

type CCImportRequest struct {
	DatabasePath string `json:"database_path"`
	Fingerprint  string `json:"fingerprint"`
	Approved     bool   `json:"approved"`
}

type CCImportSource struct {
	Platform string `json:"platform"`
	Name     string `json:"name"`
}

type CCImportKey struct {
	ID       string `json:"id"`
	Protocol string `json:"protocol"`
}

type CCImportProvider struct {
	ID       string   `json:"id"`
	BaseURLs []string `json:"base_urls"`
	Sources  []Source `json:"sources"`
	Keys     []Key    `json:"keys"`
	Status   string   `json:"status"` // ready or exists (never overwritten)
}

type CCImportModel struct {
	ID         string   `json:"id"`
	ProviderID string   `json:"provider_id"`
	Upstream   string   `json:"upstream"`
	KeyIDs     []string `json:"key_ids"`
}

type CCImportIssue struct {
	Platform string `json:"platform"`
	Name     string `json:"name"`
	Reason   string `json:"reason"`
}

type CCImportReport struct {
	DatabasePath      string     `json:"database_path"`
	Fingerprint       string     `json:"fingerprint"`
	Providers         []Provider `json:"providers"`
	Models            []Model    `json:"models"`
	Issues            []Issue    `json:"issues"`
	Failure           string     `json:"failure"` // fixed source_missing, source_invalid, source_limit
	Applied           bool       `json:"applied"`
	ImportedProviders int        `json:"imported_providers"`
	ImportedKeys      int        `json:"imported_keys"`
	ImportedModels    int        `json:"imported_models"`
}

type Plan struct {
	Report    Report
	Providers []state.ProviderImport
	Models    []state.ImportedModel
}

type Request = CCImportRequest

type Source = CCImportSource

type Key = CCImportKey

type Provider = CCImportProvider

type Model = CCImportModel

type Issue = CCImportIssue

type Report = CCImportReport
