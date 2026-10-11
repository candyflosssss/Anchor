package catalog

// Explicit DTOs shared by management, CLI and desktop. Secrets appear only in Action.
type Action struct {
	Protocol         string `json:"protocol"`
	Price            *Price `json:"price"`
	CatalogProvider  string `json:"catalog_provider"`
	ProviderID       string `json:"provider_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Action           string `json:"action"`
	Target           string `json:"target"`
	Secret           string `json:"secret"`
	Enabled          bool   `json:"enabled"`
	Approved         bool   `json:"approved"`
	CopyKeys         bool   `json:"copy_keys"`
	Strategy         string `json:"strategy"`
	Model            string `json:"model"`
	ModelRevision    int64  `json:"model_revision"`
	Tools            bool   `json:"tools"`
	Streaming        bool   `json:"streaming"`
}
type ModelProtocol struct {
	Model    string `json:"model"`
	Protocol string `json:"protocol"`
}

type KeyStatus struct {
	Protocol       string          `json:"protocol"`
	ModelProtocols []ModelProtocol `json:"model_protocols"`
	ID             string          `json:"id"`
	Enabled        bool            `json:"enabled"`
	State          string          `json:"state"`
	Failures       int             `json:"failures"`
	RetryAt        int64           `json:"retry_at"`
	Test           *Probe          `json:"test"`
}
type Probe struct {
	Revision   int64  `json:"revision"`
	KeyID      string `json:"key_id"`
	Model      string `json:"model"`
	At         int64  `json:"at"`
	HTTPStatus int    `json:"http_status"`
	ElapsedMS  int64  `json:"elapsed_ms"`
	Failure    string `json:"failure"`
}
type ModelMapping struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	Upstream       string `json:"upstream"`
	Tools          bool   `json:"tools"`
	Streaming      bool   `json:"streaming"`
	Revision       int64  `json:"revision"`
	CandidateCount int    `json:"candidate_count"`
}
type UsageRow struct {
	ProviderID   string `json:"provider_id"`
	Model        string `json:"model"`
	Requests     int64  `json:"requests"`
	Failures     int64  `json:"failures"`
	ElapsedMS    int64  `json:"elapsed_ms"`
	Known        int64  `json:"known"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read"`
	CacheWrite   int64  `json:"cache_write"`
}
type Workspace struct {
	Prices        []Price        `json:"prices"`
	Estimates     []Estimate     `json:"estimates"`
	ImportedPrice *Price         `json:"imported_price"`
	ProviderID    string         `json:"provider_id"`
	Revision      int64          `json:"revision"`
	Strategy      string         `json:"strategy"`
	Keys          []KeyStatus    `json:"keys"`
	Models        []ModelMapping `json:"models"`
	Usage         []UsageRow     `json:"usage"`
	Probe         *Probe         `json:"probe"`
	Deleted       bool           `json:"deleted"`
}

type Price struct {
	Model         string   `json:"model"`
	InputUSD      float64  `json:"input_usd"`
	OutputUSD     float64  `json:"output_usd"`
	CacheReadUSD  *float64 `json:"cache_read_usd"`
	CacheWriteUSD *float64 `json:"cache_write_usd"`
	Source        string   `json:"source"`
	UpdatedAt     int64    `json:"updated_at"`
}
type Estimate struct {
	Model string   `json:"model"`
	USD   *float64 `json:"usd"`
}
