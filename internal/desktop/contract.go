// Package desktop is the platform-independent, typed desktop boundary.
package desktop

import (
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

const Contract = "anchor-desktop-v1"
const MaxSafeRevision int64 = 9007199254740991

// ProblemCodes is the sole source for generated TypeScript error literals.
var ProblemCodes = []string{"offline", "unauthorized", "read_error", "conflict", "unknown", "invalid", "startup_failed", "identity", "unsupported", "closed", "integrity", "permission", "referenced", "ssh_missing", "ssh_host_key", "ssh_auth", "ssh_unreachable", "remote_anchor", "remote_anchor_missing", "remote_anchor_unsupported", "remote_control_port", "remote_core_not_ready", "tunnel_failed", "agent_operation_failed", "agent_model_unavailable", "agent_catalog_invalid", "agent_config_conflict", "agent_config_unsafe", "skill_unsafe", "skill_changed", "skill_limit", "skill_link_unavailable"}

type Problem struct {
	Code string `json:"code"`
}
type Provider struct {
	CostMultiplier float64  `json:"cost_multiplier"`
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
	ProfileName string `json:"profile_name"`
	Transport   string `json:"transport"`
	Target      string `json:"target"`
	InstanceID  string `json:"instance_id"`
	Phase       string `json:"phase"`
	Source      string `json:"source"`
	CanStop     bool   `json:"can_stop"`
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
type ModelEntry = catalog.ModelEntry
type CatalogKey = catalog.CatalogKey
type ProviderModels = catalog.ProviderModels
type ProviderModelsResult struct {
	Contract string          `json:"contract"`
	Data     *ProviderModels `json:"data"`
	Problem  *Problem        `json:"problem"`
}

type Action = catalog.Action
type ModelProtocol = catalog.ModelProtocol
type KeyRateCache = catalog.KeyRateCache
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
type LogReferenceCost = logbook.ReferenceCost
type LogMetrics = logbook.Metrics
type LogAttempt = logbook.Attempt
type RequestLog = logbook.Request
type SystemLog = logbook.Event
type LogSuggestionsQuery = logbook.SuggestionsQuery
type LogSuggestion = logbook.Suggestion
type LogSuggestions = logbook.Suggestions
type LogSuggestionsResult struct {
	Contract string          `json:"contract"`
	Data     *LogSuggestions `json:"data"`
	Problem  *Problem        `json:"problem"`
}
type LogQuery = logbook.Query
type LogPage = logbook.Page
type LogHealth = logbook.Health
type LogSettings = logbook.Settings
type LogStatsQuery = logbook.StatsQuery
type LogStatsValues = logbook.StatsValues
type LogStatsGroup = logbook.StatsGroup
type LogStatsBucket = logbook.StatsBucket
type LogStats = logbook.Stats
type LogStatsResult struct {
	Contract string    `json:"contract"`
	Data     *LogStats `json:"data"`
	Problem  *Problem  `json:"problem"`
}
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

// Account DTOs expose public configuration and decimal strings only. Credentials
// are write-only; account wallets and inference-key caps retain distinct scopes.
type AccountSettings struct {
	ProviderID   string `json:"provider_id"`
	Revision     int64  `json:"revision"`
	Platform     string `json:"platform"`
	SiteURL      string `json:"site_url"`
	UserID       int64  `json:"user_id"`
	HasUserToken bool   `json:"has_user_token"`
}
type SaveAccountSettings struct {
	ProviderID       string  `json:"provider_id"`
	ExpectedRevision int64   `json:"expected_revision"`
	Platform         string  `json:"platform"`
	SiteURL          string  `json:"site_url"`
	UserID           int64   `json:"user_id"`
	UserToken        *string `json:"user_token"`
}
type AccountQuery struct {
	ProviderID       string `json:"provider_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	// Operation is balance, key-quota, balance-summary, or effective-rate (via ProviderRate).
	Operation   string `json:"operation"`
	KeyID       string `json:"key_id"`
	RemoteKeyID int64  `json:"remote_key_id"`
}

// AccountAmount keeps USD/CNY, raw quota, or site_unit when currency is unknown.
type AccountAmount struct {
	Value string `json:"value"`
	Unit  string `json:"unit"`
}
type AccountWindow struct {
	Period    string         `json:"period"`
	Remaining *AccountAmount `json:"remaining"`
	Used      *AccountAmount `json:"used"`
	Limit     *AccountAmount `json:"limit"`
	ResetAt   string         `json:"reset_at"`
}

// AccountAllowance scope is account, key, or billing (site-controlled scope).
type AccountAllowance struct {
	Scope     string          `json:"scope"`
	Remaining *AccountAmount  `json:"remaining"`
	Used      *AccountAmount  `json:"used"`
	Limit     *AccountAmount  `json:"limit"`
	Unlimited *bool           `json:"unlimited"`
	Windows   []AccountWindow `json:"windows"`
}
type AccountObservation struct {
	ProviderID  string            `json:"provider_id"`
	Revision    int64             `json:"revision"`
	Platform    string            `json:"platform"`
	Operation   string            `json:"operation"`
	KeyID       string            `json:"key_id"`
	RemoteKeyID int64             `json:"remote_key_id"`
	ObservedAt  string            `json:"observed_at"`
	HTTPStatus  int               `json:"http_status"`
	Failure     string            `json:"failure"`
	Allowance   *AccountAllowance `json:"allowance"`
}
type AccountSettingsResult struct {
	Contract string           `json:"contract"`
	Data     *AccountSettings `json:"data"`
	Problem  *Problem         `json:"problem"`
}
type AccountObservationResult struct {
	Contract string              `json:"contract"`
	Data     *AccountObservation `json:"data"`
	Problem  *Problem            `json:"problem"`
}

// Platform rates are observations of a selected Key's group, independent of
// the saved model mapping's routing multiplier. A missing rate is never 1.
// Basis key_billing includes user/peak rules; the endpoint may omit group identity.
type AccountRate struct {
	Group      string  `json:"group"`
	GroupName  string  `json:"group_name"`
	Multiplier *string `json:"multiplier"`
	Basis      string  `json:"basis"`
}
type AccountRateObservation struct {
	ProviderID  string       `json:"provider_id"`
	Revision    int64        `json:"revision"`
	Platform    string       `json:"platform"`
	Operation   string       `json:"operation"`
	KeyID       string       `json:"key_id"`
	RemoteKeyID int64        `json:"remote_key_id"`
	ObservedAt  string       `json:"observed_at"`
	HTTPStatus  int          `json:"http_status"`
	Failure     string       `json:"failure"`
	Rate        *AccountRate `json:"rate"`
}
type AccountRateObservationResult struct {
	Contract string                  `json:"contract"`
	Data     *AccountRateObservation `json:"data"`
	Problem  *Problem                `json:"problem"`
}

// The transient inference lifecycle uses the same core probe as CLI tests.
type ProbeRequest = catalog.ProbeRequest
type ProbeProgress = catalog.ProbeProgress
type ProbeProgressResult struct {
	Contract string         `json:"contract"`
	Data     *ProbeProgress `json:"data"`
	Problem  *Problem       `json:"problem"`
}

// Model routing writes preserve candidate membership and all candidate metadata.
type RouteCandidate = catalog.RouteCandidate
type ModelRouting = catalog.ModelRouting
type ModelRoutingState = catalog.ModelRoutingState
type ModelRoutingResult struct {
	Contract string             `json:"contract"`
	Data     *ModelRoutingState `json:"data"`
	Problem  *Problem           `json:"problem"`
}

// Model reference data is read-only, separate from actual provider availability.
type ModelCatalogQuery = modeldev.ModelCatalogQuery
type CatalogProvider = modeldev.CatalogProvider
type CatalogCapabilities = modeldev.CatalogCapabilities
type CatalogLimits = modeldev.CatalogLimits
type ReasoningOption = modeldev.ReasoningOption
type CatalogRates = modeldev.CatalogRates
type CatalogTier = modeldev.CatalogTier
type CatalogPricing = modeldev.CatalogPricing
type ContextPolicy = modeldev.ContextPolicy
type CatalogModel = modeldev.CatalogModel
type ModelCatalog = modeldev.ModelCatalog
type CatalogUsage = modeldev.CatalogUsage
type CatalogEstimateInput = modeldev.CatalogEstimateInput
type CatalogEstimate = modeldev.CatalogEstimate
type ModelCatalogResult struct {
	Contract string        `json:"contract"`
	Data     *ModelCatalog `json:"data"`
	Problem  *Problem      `json:"problem"`
}
type CatalogEstimateResult struct {
	Contract string           `json:"contract"`
	Data     *CatalogEstimate `json:"data"`
	Problem  *Problem         `json:"problem"`
}

// Mapping settings use a scoped revision-controlled write.
type ReasoningDefault = state.ReasoningDefault
type ModelConfiguration = state.ModelConfiguration
type ModelSettingsQuery = catalog.ModelSettingsQuery
type ModelSettingsState = catalog.ModelSettingsState
type SaveModelSettings = catalog.SaveModelSettings
type ModelSettingsResult struct {
	Contract string              `json:"contract"`
	Data     *ModelSettingsState `json:"data"`
	Problem  *Problem            `json:"problem"`
}

// Connection DTOs contain only public profile data; credentials stay in Go.
type ConnectionProfile struct {
	Authentication string `json:"authentication"`
	Name           string `json:"name"`
	Transport      string `json:"transport"`
	Target         string `json:"target"`
	InstanceID     string `json:"instance_id"`
	Active         bool   `json:"active"`
}
type SSHConnectionInput struct {
	Password          string `json:"password"`
	Name              string `json:"name"`
	Target            string `json:"target"`
	Port              int    `json:"port"`
	IdentityFile      string `json:"identity_file"`
	RemoteExecutable  string `json:"remote_executable"`
	RemoteControlPort int    `json:"remote_control_port"`
	TrustNewHost      bool   `json:"trust_new_host"`
}
type SelectConnection struct {
	Password string `json:"password"`
	Name     string `json:"name"`
}
type ConnectionProfilesResult struct {
	Contract string              `json:"contract"`
	Data     []ConnectionProfile `json:"data"`
	Problem  *Problem            `json:"problem"`
}

// Text-only conversation tests use the shared core validation and transient progress.
type ChatMessage = catalog.ChatMessage
type ChatRequest = catalog.ChatRequest

// Proxy DTOs are shared with the core and CLI; secrets are write-only.
type ProxyProfile = state.ProxyProfile
type ProxyBinding = state.ProxyBinding
type ProxyTarget = state.ProxyTarget
type ProxyEngine = state.ProxyEngine
type ProxyWorkspace = state.ProxyWorkspace
type ProxyAction = state.ProxyAction
type ProxyWorkspaceResult struct {
	Contract string          `json:"contract"`
	Data     *ProxyWorkspace `json:"data"`
	Problem  *Problem        `json:"problem"`
}

type CodexSettings = state.CodexSettings
type SaveCodexSettings = state.SaveCodexSettings
type CodexSettingsState = managed.CodexSettingsState
type CodexSettingsResult struct {
	Contract string              `json:"contract"`
	Data     *CodexSettingsState `json:"data"`
	Problem  *Problem            `json:"problem"`
}
