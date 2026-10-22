// Package apierror owns Anchor API error identities, HTTP status codes and localized messages.
// Error codes remain stable across languages; IDs distinguish messages sharing a code.
package apierror

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

type ID string

const (
	Referenced                     ID = "referenced"
	Invalid                        ID = "invalid"
	Conflict                       ID = "conflict"
	NotFound                       ID = "not_found"
	Internal                       ID = "internal"
	InvalidJSONFields              ID = "invalid_json_fields"
	SingleJSONObject               ID = "single_json_object"
	BrowserOriginRejected          ID = "browser_origin_rejected"
	InvalidHost                    ID = "invalid_host"
	Unauthorized                   ID = "unauthorized"
	Starting                       ID = "starting"
	Stopping                       ID = "stopping"
	ShutdownObjectRequired         ID = "shutdown_object_required"
	RestartBooleanRequired         ID = "restart_boolean_required"
	NotRunning                     ID = "not_running"
	ShutdownModeConflict           ID = "shutdown_mode_conflict"
	InvalidID                      ID = "invalid_id"
	InvalidProvider                ID = "invalid_provider"
	InvalidModelConfiguration      ID = "invalid_model_configuration"
	InvalidCandidate               ID = "invalid_candidate"
	InstanceMismatch               ID = "instance_mismatch"
	Busy                           ID = "busy"
	CatalogUnavailable             ID = "catalog_unavailable"
	CatalogRefreshFailed           ID = "catalog_refresh_failed"
	SkillUnsafe                    ID = "skill_unsafe"
	SkillChanged                   ID = "skill_changed"
	SkillLimit                     ID = "skill_limit"
	SkillLinkUnavailable           ID = "skill_link_unavailable"
	InvalidRouting                 ID = "invalid_routing"
	CandidateMembershipChanged     ID = "candidate_membership_changed"
	UnknownCandidate               ID = "unknown_candidate"
	AgentOperationFailed           ID = "agent_operation_failed"
	AgentModelUnavailable          ID = "agent_model_unavailable"
	AgentCatalogInvalid            ID = "agent_catalog_invalid"
	AgentConfigConflict            ID = "agent_config_conflict"
	AgentConfigUnsafe              ID = "agent_config_unsafe"
	ApprovalRequired               ID = "approval_required"
	InvalidModelPolicy             ID = "invalid_model_policy"
	PlanRequired                   ID = "plan_required"
	UnknownAgentAction             ID = "unknown_agent_action"
	InvalidLogQuery                ID = "invalid_log_query"
	LogNotFound                    ID = "log_not_found"
	LogStorageUnavailable          ID = "log_storage_unavailable"
	ProviderIDMismatch             ID = "provider_id_mismatch"
	InvalidPlatformQuery           ID = "invalid_platform_query"
	TieredPrice                    ID = "tiered_price"
	ProviderRevisionRequired       ID = "provider_revision_required"
	RecoveryTimeout                ID = "recovery_timeout"
	GatewayBusy                    ID = "gateway_busy"
	RequestTooLarge                ID = "request_too_large"
	RequestObjectRequired          ID = "request_object_required"
	ModelIDRequired                ID = "model_id_required"
	BindingModelDenied             ID = "binding_model_denied"
	ModelNoEnabledProvider         ID = "model_no_enabled_provider"
	UpstreamHeaderTimeout          ID = "upstream_header_timeout"
	UpstreamFirstTokenTimeout      ID = "upstream_first_token_timeout"
	InvalidStream                  ID = "invalid_stream"
	PreviousResponseUnsupported    ID = "previous_response_unsupported"
	ConversationUnsupported        ID = "conversation_unsupported"
	StoreUnsupported               ID = "store_unsupported"
	BackgroundUnsupported          ID = "background_unsupported"
	UnknownModel                   ID = "unknown_model"
	StreamingUnsupported           ID = "streaming_unsupported"
	InvalidTools                   ID = "invalid_tools"
	ToolsUnsupported               ID = "tools_unsupported"
	UnsupportedTool                ID = "unsupported_tool"
	NoAvailableCandidate           ID = "no_available_candidate"
	UnsupportedConversion          ID = "unsupported_conversion"
	CandidateConversionUnsupported ID = "candidate_conversion_unsupported"
	ProxyUnavailable               ID = "proxy_unavailable"
	UpstreamTimeout                ID = "upstream_timeout"
	UpstreamTransport              ID = "upstream_transport"
	UpstreamRejected               ID = "upstream_rejected"
	UnexpectedHTML                 ID = "unexpected_html"
	UpstreamContentType            ID = "upstream_content_type"
	UpstreamIncomplete             ID = "upstream_incomplete"
	UpstreamInvalidJSON            ID = "upstream_invalid_json"
	UpstreamFailed                 ID = "upstream_failed"
	UpstreamUnsupportedContent     ID = "upstream_unsupported_content"
	UpstreamMissingUsage           ID = "upstream_missing_usage"
	UpstreamStreamIncomplete       ID = "upstream_stream_incomplete"
)

type definition struct {
	status    int
	code      string
	arguments string
}

var definitions = map[ID]definition{
	UpstreamStreamIncomplete:       {502, "upstream_stream_error", ""},
	Referenced:                     {409, "referenced", ""},
	Invalid:                        {400, "invalid", ""},
	Conflict:                       {409, "conflict", ""},
	NotFound:                       {404, "not_found", ""},
	Internal:                       {500, "internal_error", ""},
	InvalidJSONFields:              {400, "invalid_json", ""},
	SingleJSONObject:               {400, "invalid_json", ""},
	BrowserOriginRejected:          {403, "browser_origin_rejected", ""},
	InvalidHost:                    {400, "invalid_host", ""},
	Unauthorized:                   {401, "unauthorized", ""},
	Starting:                       {503, "starting", ""},
	Stopping:                       {503, "stopping", ""},
	ShutdownObjectRequired:         {400, "invalid_json", ""},
	RestartBooleanRequired:         {400, "invalid_json", ""},
	NotRunning:                     {503, "not_running", ""},
	ShutdownModeConflict:           {409, "conflict", ""},
	InvalidID:                      {400, "invalid_id", ""},
	InvalidProvider:                {400, "invalid_provider", ""},
	InvalidModelConfiguration:      {400, "invalid_model", ""},
	InvalidCandidate:               {400, "invalid_candidate", ""},
	InstanceMismatch:               {409, "instance_mismatch", ""},
	Busy:                           {503, "busy", ""},
	CatalogUnavailable:             {502, "catalog_unavailable", ""},
	CatalogRefreshFailed:           {502, "catalog_unavailable", ""},
	SkillUnsafe:                    {409, "skill_unsafe", ""},
	SkillChanged:                   {409, "skill_changed", ""},
	SkillLimit:                     {400, "skill_limit", ""},
	SkillLinkUnavailable:           {409, "skill_link_unavailable", ""},
	InvalidRouting:                 {400, "invalid", ""},
	CandidateMembershipChanged:     {400, "invalid", ""},
	UnknownCandidate:               {400, "invalid", ""},
	AgentOperationFailed:           {409, "agent_operation_failed", ""},
	AgentModelUnavailable:          {409, "agent_model_unavailable", ""},
	AgentCatalogInvalid:            {409, "agent_catalog_invalid", ""},
	AgentConfigConflict:            {409, "agent_config_conflict", ""},
	AgentConfigUnsafe:              {409, "agent_config_unsafe", ""},
	ApprovalRequired:               {400, "approval_required", ""},
	InvalidModelPolicy:             {400, "invalid_model_policy", ""},
	PlanRequired:                   {400, "plan_required", ""},
	UnknownAgentAction:             {404, "not_found", ""},
	InvalidLogQuery:                {400, "invalid_log_query", ""},
	LogNotFound:                    {404, "log_not_found", ""},
	LogStorageUnavailable:          {503, "log_storage_unavailable", ""},
	ProviderIDMismatch:             {400, "invalid_request", ""},
	InvalidPlatformQuery:           {400, "invalid_request", ""},
	TieredPrice:                    {400, "invalid", ""},
	ProviderRevisionRequired:       {400, "invalid_request", ""},
	RecoveryTimeout:                {504, "upstream_timeout", ""},
	GatewayBusy:                    {429, "gateway_busy", ""},
	RequestTooLarge:                {413, "request_too_large", "d"},
	RequestObjectRequired:          {400, "invalid_json", ""},
	ModelIDRequired:                {400, "invalid_model", ""},
	BindingModelDenied:             {403, "binding_model_denied", ""},
	ModelNoEnabledProvider:         {503, "model_no_enabled_provider", ""},
	UpstreamHeaderTimeout:          {504, "upstream_header_timeout", ""},
	UpstreamFirstTokenTimeout:      {504, "upstream_first_token_timeout", ""},
	InvalidStream:                  {400, "invalid_stream", ""},
	PreviousResponseUnsupported:    {400, "unsupported_state", ""},
	ConversationUnsupported:        {400, "unsupported_state", ""},
	StoreUnsupported:               {400, "unsupported_state", ""},
	BackgroundUnsupported:          {400, "unsupported_background", ""},
	UnknownModel:                   {404, "unknown_model", ""},
	StreamingUnsupported:           {400, "unsupported_capability", ""},
	InvalidTools:                   {400, "invalid_tools", ""},
	ToolsUnsupported:               {400, "unsupported_capability", ""},
	UnsupportedTool:                {400, "unsupported_tool", ""},
	NoAvailableCandidate:           {503, "no_available_candidate", ""},
	UnsupportedConversion:          {400, "unsupported_conversion", ""},
	CandidateConversionUnsupported: {400, "unsupported_conversion", "ss"},
	ProxyUnavailable:               {502, "proxy_unavailable", ""},
	UpstreamTimeout:                {504, "upstream_timeout", ""},
	UpstreamTransport:              {502, "upstream_transport", ""},
	UpstreamRejected:               {502, "upstream_rejected", ""},
	UnexpectedHTML:                 {502, "unexpected_html", ""},
	UpstreamContentType:            {502, "invalid_upstream_response", ""},
	UpstreamIncomplete:             {502, "invalid_upstream_response", ""},
	UpstreamInvalidJSON:            {502, "invalid_upstream_response", ""},
	UpstreamFailed:                 {502, "upstream_failed", ""},
	UpstreamUnsupportedContent:     {502, "unsupported_upstream_response", ""},
	UpstreamMissingUsage:           {502, "unsupported_upstream_response", ""},
}

// Detail is the existing wire error shape. It never includes an underlying error.
type Detail struct {
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	Language Language `json:"-"` // Actual message language, for Content-Language.
}

//go:embed locales/*.json
var localeFiles embed.FS

var messages = loadMessages()

func loadMessages() map[Language]map[ID]string {
	out := map[Language]map[ID]string{}
	for _, lang := range []Language{English, Chinese} {
		raw, err := localeFiles.ReadFile("locales/" + string(lang) + ".json")
		if err != nil {
			panic(err)
		}
		var catalog map[ID]string
		if err := json.Unmarshal(raw, &catalog); err != nil {
			panic(err)
		}
		out[lang] = catalog
	}
	return out
}

// Render returns a catalog-owned status/code/message, falling back to English.
// Arguments are reserved for safe, structured facts (limits/protocol/reason tags),
// never raw validation, filesystem, parser or upstream error strings.
func Render(id ID, lang Language, args ...any) (int, Detail) {
	d, ok := definitions[id]
	if !ok || !validArguments(d.arguments, args) {
		id, d, args = Internal, definitions[Internal], nil
	}
	message := messages[lang][id]
	if message == "" {
		lang = English
		message = messages[English][id]
	}
	if message == "" {
		message = messages[English][Internal]
	}
	if len(args) > 0 {
		message = fmt.Sprintf(message, args...)
	}
	return d.status, Detail{Code: d.code, Message: "[Anchor] " + message, Language: lang}
}

func validArguments(kinds string, args []any) bool {
	if len(kinds) != len(args) {
		return false
	}
	for i, kind := range kinds {
		switch kind {
		case 'd':
			v, ok := args[i].(int)
			if !ok || v < 0 {
				return false
			}
		case 's':
			v, ok := args[i].(string)
			if !ok || len(v) > 128 || strings.ContainsAny(v, "\r\n") {
				return false
			}
		default:
			return false
		}
	}
	return true
}
