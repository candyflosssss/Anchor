// Package desktop is the platform-independent, typed desktop boundary.
package desktop

import (
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/managed"
)

const Contract = "anchor-desktop-v1"
const MaxSafeRevision int64 = 9007199254740991

// ProblemCodes is the sole source for generated TypeScript error literals.
var ProblemCodes = []string{"offline", "unauthorized", "read_error", "conflict", "unknown", "invalid", "startup_failed", "identity", "unsupported", "closed", "integrity", "permission", "referenced"}

type Problem struct {
	Code string `json:"code"`
}
type Provider struct {
	ID             string   `json:"id"`
	HomepageURL    string   `json:"homepage_url"`
	BaseURLs       []string `json:"base_urls"`
	Enabled        bool     `json:"enabled"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	MaxConcurrent  int      `json:"max_concurrent"`
	Revision       int64    `json:"revision"`
	HasSecret      bool     `json:"has_secret"`
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

type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	Channel        string `json:"channel"`
	Source         string `json:"source"`
	Available      bool   `json:"available"`
	Phase          string `json:"phase"`
	DownloadPath   string `json:"download_path"`
	InstallKind    string `json:"install_kind"`
}
type ConfirmUpdate struct {
	Version  string `json:"version"`
	Approved bool   `json:"approved"`
}
type UpdateResult struct {
	Contract string      `json:"contract"`
	Data     *UpdateInfo `json:"data"`
	Problem  *Problem    `json:"problem"`
}

// Discovery DTOs share the core's canonical read-only contract.
type DiscoverProviderModels = catalog.DiscoverProviderModels
type ProviderModels = catalog.ProviderModels
type ProviderModelsResult struct {
	Contract string          `json:"contract"`
	Data     *ProviderModels `json:"data"`
	Problem  *Problem        `json:"problem"`
}

type Action = catalog.Action
type ModelProtocol = catalog.ModelProtocol
type KeyStatus = catalog.KeyStatus
type Probe = catalog.Probe
type ModelMapping = catalog.ModelMapping
type UsageRow = catalog.UsageRow
type Workspace = catalog.Workspace
type WorkspaceQuery struct {
	ProviderID string `json:"provider_id"`
}
type WorkspaceResult struct {
	Contract string     `json:"contract"`
	Data     *Workspace `json:"data"`
	Problem  *Problem   `json:"problem"`
}

type Price = catalog.Price
type Estimate = catalog.Estimate

// Logging DTOs use the canonical body-free core contract.
type LogMetrics = logbook.Metrics
type LogAttempt = logbook.Attempt
type RequestLog = logbook.Request
type SystemLog = logbook.Event
type LogQuery = logbook.Query
type LogPage = logbook.Page
type LogHealth = logbook.Health
type LogSettings = logbook.Settings
type LogsResult struct {
	Contract string   `json:"contract"`
	Data     *LogPage `json:"data"`
	Problem  *Problem `json:"problem"`
}
type LogDetailResult struct {
	Contract string      `json:"contract"`
	Data     *RequestLog `json:"data"`
	Problem  *Problem    `json:"problem"`
}
type LogSettingsResult struct {
	Contract string       `json:"contract"`
	Data     *LogSettings `json:"data"`
	Problem  *Problem     `json:"problem"`
}

// Agent setup's core DTOs exclude credentials, full instruction templates and
// arbitrary filesystem arguments; desktop adds only its standard envelope.
type CatalogTemplate = managed.CatalogTemplate
type AgentTarget = managed.AgentTarget
type AgentModel = managed.AgentModel
type ManagedAgent = managed.ManagedAgent
type AgentWorkspace = managed.AgentWorkspace
type EnrollAgent = managed.EnrollAgent
type ManageAgent = managed.ManageAgent
type AgentWorkspaceResult struct {
	Contract string          `json:"contract"`
	Data     *AgentWorkspace `json:"data"`
	Problem  *Problem        `json:"problem"`
}
