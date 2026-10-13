// Package providerapi normalizes read-only account and billing APIs. It never
// treats an inference protocol as a platform, or a key allowance as a wallet.
package providerapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Platform string `json:"platform"`
	// SiteURL is the dashboard deployment root, including any proxy prefix.
	// Empty derives it by removing a final /v1 from the inference BaseURL.
	SiteURL      string `json:"site_url,omitempty"`
	UserID       int64  `json:"user_id,omitempty"`
	HasUserToken bool   `json:"has_user_token"`
	UserToken    string `json:"-"`
}

func (c Config) Validate() error {
	if c.Platform != "" && c.Platform != "auto" && c.Platform != "sub2api" && c.Platform != "newapi" && c.Platform != "oneapi" {
		return errors.New("platform must be auto, sub2api, newapi or oneapi")
	}
	if c.UserID < 0 || len(c.UserToken) > 8192 || strings.ContainsAny(c.UserToken, "\r\n") {
		return errors.New("invalid user authorization")
	}
	if c.SiteURL != "" {
		u, e := url.Parse(c.SiteURL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("site_url must be an HTTP(S) deployment root without credentials, query or fragment")
		}
	}
	return nil
}

type Settings struct {
	ProviderID string `json:"provider_id"`
	Revision   int64  `json:"revision"`
	Config     Config `json:"config"`
}
type Configure struct {
	ProviderID       string `json:"provider_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Config           Config `json:"config"`
	// nil keeps an existing credential, empty clears it. Never returned.
	UserToken *string `json:"user_token,omitempty"`
}

type Query struct {
	ProviderID       string `json:"provider_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Operation        string `json:"operation"`
	KeyID            string `json:"key_id,omitempty"`
	// RemoteKeyID is the site's numeric key identifier, not the secret/local ID.
	RemoteKeyID int64 `json:"remote_key_id,omitempty"`
	// Scope is account (default) or key for records. Key scope on sub2api/oneapi
	// requires RemoteKeyID (original OneAPI cannot filter reliably); NewAPI uses the selected inference key directly.
	Scope    string `json:"scope,omitempty"`
	Page     int    `json:"page,omitempty"` // all public pagination is one-based
	PageSize int    `json:"page_size,omitempty"`
}

func (q Query) Validate() error {
	switch q.Operation {
	case "models", "detect-platform", "balance-summary", "balance", "key-quota", "usage-records", "effective-rate":
	default:
		return errors.New("unsupported operation")
	}
	if q.ExpectedRevision < 1 || q.RemoteKeyID < 0 || q.Page < 0 || q.Page > 100000 || q.PageSize < 0 || q.PageSize > 100 || (q.Scope != "" && q.Scope != "account" && q.Scope != "key") {
		return errors.New("invalid provider query")
	}
	return nil
}

type Amount struct {
	Value json.Number `json:"value"`
	Unit  string      `json:"unit"` // USD, or quota (no guessed currency conversion)
}
type Allowance struct {
	Scope     string   `json:"scope"` // account or key, never inferred from a field name
	Remaining *Amount  `json:"remaining"`
	Used      *Amount  `json:"used"`
	Limit     *Amount  `json:"limit"`
	Unlimited *bool    `json:"unlimited"`
	Windows   []Window `json:"windows"`
}
type Window struct {
	Period    string  `json:"period"`
	Remaining *Amount `json:"remaining"`
	Used      *Amount `json:"used"`
	Limit     *Amount `json:"limit"`
	ResetAt   string  `json:"reset_at,omitempty"`
}
type Tokens struct {
	Input        *int64 `json:"input"`
	Output       *int64 `json:"output"`
	CacheRead    *int64 `json:"cache_read"`
	CacheWrite   *int64 `json:"cache_write"`
	CacheWrite5m *int64 `json:"cache_write_5m"`
	CacheWrite1h *int64 `json:"cache_write_1h"`
	Semantics    string `json:"semantics"` // reported_tokens or platform_fields_unverified
}
type ToolCharge struct {
	Name  string       `json:"name"`
	Count *int64       `json:"count"`
	Price *json.Number `json:"price"` // raw site tool price; no currency conversion
}

type Record struct {
	ID             string       `json:"id,omitempty"`
	IDStable       bool         `json:"id_stable"`
	RequestID      string       `json:"request_id,omitempty"`
	Model          string       `json:"model"`
	CreatedAt      string       `json:"created_at"`
	Group          string       `json:"group,omitempty"`
	RemoteKeyID    string       `json:"remote_key_id,omitempty"`
	Tokens         Tokens       `json:"tokens"`
	ChargedAmount  *Amount      `json:"charged_amount"`
	BaseAmount     *Amount      `json:"base_amount"`
	BillingSource  string       `json:"billing_source,omitempty"`
	ToolCharges    []ToolCharge `json:"tool_charges,omitempty"`
	RateMultiplier *json.Number `json:"rate_multiplier"`
	// Only known numeric billing fields are exposed. Raw log bodies can contain
	// prompts, IPs or secrets and must never be copied to management responses.
	Details map[string]json.Number `json:"details"`
}
type Records struct {
	Scope    string   `json:"scope"`
	History  string   `json:"history"` // recent or paginated_retained
	Page     int      `json:"page"`
	PageSize int      `json:"page_size"`
	Total    *int64   `json:"total"`
	HasMore  bool     `json:"has_more"`
	Items    []Record `json:"items"`
}
type Rate struct {
	GroupName  string       `json:"group_name,omitempty"`
	Group      string       `json:"group"`
	Multiplier *json.Number `json:"multiplier"`
	Basis      string       `json:"basis"` // personal_override or authenticated_group
}
type Result struct {
	ProviderID    string     `json:"provider_id"`
	KeyID         string     `json:"key_id,omitempty"`
	RemoteKeyID   int64      `json:"remote_key_id,omitempty"`
	Revision      int64      `json:"revision"`
	Platform      string     `json:"platform"`
	Operation     string     `json:"operation"`
	ObservedAt    string     `json:"observed_at"`
	Source        []string   `json:"source"` // endpoint paths, never credentials or URLs
	HTTPStatus    int        `json:"http_status"`
	Failure       string     `json:"failure,omitempty"`
	Models        []string   `json:"models,omitempty"`
	Balance       *Allowance `json:"balance,omitempty"`
	KeyQuota      *Allowance `json:"key_quota,omitempty"`
	Records       *Records   `json:"records,omitempty"`
	EffectiveRate *Rate      `json:"effective_rate,omitempty"`
}

type Failure struct{ Code string }

func (e *Failure) Error() string { return e.Code }
func failure(code string) error  { return &Failure{code} }

type Adapter struct {
	HTTP    *http.Client
	Config  Config
	BaseURL string
	Key     string
	paths   []string
	status  int
	calls   int
	bytes   int
}

func (a *Adapter) root() string {
	if a.Config.SiteURL != "" {
		return strings.TrimRight(a.Config.SiteURL, "/")
	}
	return strings.TrimSuffix(strings.TrimRight(a.BaseURL, "/"), "/v1")
}
func (a *Adapter) user() error {
	if a.Config.UserToken == "" {
		return failure("auth_required")
	}
	return nil
}
func (a *Adapter) key() error {
	if a.Key == "" {
		return failure("auth_required")
	}
	return nil
}

// Query resets a bounded operation. A single adapter is owned by one query.
func (a *Adapter) Query(ctx context.Context, q Query) Result {
	return a.query(ctx, q, false)
}

// QueryKeyRate resolves this saved inference credential against the authorized
// account before observing its current group. It never guesses from a local ID.
func (a *Adapter) QueryKeyRate(ctx context.Context) Result {
	if a.Config.Platform == "" && a.Config.UserToken == "" {
		return Result{Failure: "platform_required"}
	}
	return a.query(ctx, Query{Operation: "effective-rate", ExpectedRevision: 1}, true)
}

func (a *Adapter) query(ctx context.Context, q Query, resolveKey bool) Result {
	out := Result{ProviderID: q.ProviderID, KeyID: q.KeyID, RemoteKeyID: q.RemoteKeyID, Revision: q.ExpectedRevision, Platform: a.Config.Platform, Operation: q.Operation, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Source: []string{}}
	if err := q.Validate(); err != nil || a.Config.Validate() != nil {
		out.Failure = "invalid_request"
		return out
	}
	a.paths = nil
	a.status = 0
	a.calls = 0
	a.bytes = 0
	var err error
	if q.Operation == "detect-platform" || a.Config.Platform == "" || a.Config.Platform == "auto" {
		a.Config.Platform, err = a.DetectPlatform(ctx)
		out.Platform = a.Config.Platform
	}
	if err == nil {
		switch q.Operation {
		case "detect-platform":
		case "balance-summary":
			out.Balance, err = a.GetBalanceSummary(ctx)
		case "balance":
			out.Balance, err = a.GetBalance(ctx)
		case "key-quota":
			out.KeyQuota, err = a.GetKeyQuota(ctx, q.RemoteKeyID)
		case "usage-records":
			out.Records, err = a.ListUsageRecords(ctx, q)
		case "effective-rate":
			if resolveKey {
				out.EffectiveRate, err = a.getKeyRate(ctx)
			} else {
				out.EffectiveRate, err = a.GetEffectiveRate(ctx, q.RemoteKeyID)
			}
		default:
			err = failure("unsupported") // models uses the shared catalog reader in core
		}
	}
	out.Source = append(out.Source, a.paths...)
	out.HTTPStatus = a.status
	if err != nil {
		out.Balance = nil
		out.KeyQuota = nil
		out.Records = nil
		if !resolveKey {
			out.EffectiveRate = nil
		} else if out.EffectiveRate != nil {
			out.EffectiveRate.Multiplier = nil
		}
		var f *Failure
		if errors.As(err, &f) {
			out.Failure = f.Code
		} else {
			out.Failure = "invalid_response"
		}
	}
	// Defend against an upstream echoing either credential into allowed strings.
	raw, _ := json.Marshal(out)
	for _, secret := range []string{a.Key, a.Config.UserToken} {
		encoded, _ := json.Marshal(secret)
		if secret != "" && strings.Contains(string(raw), string(encoded[1:len(encoded)-1])) {
			out.Balance = nil
			out.KeyQuota = nil
			out.Records = nil
			out.EffectiveRate = nil
			out.Failure = "invalid_response"
		}
	}
	return out
}
func remoteID(id int64) string { return strconv.FormatInt(id, 10) }
