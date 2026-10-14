package state

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/candyflosssss/Anchor/internal/naming"
)

var ErrConflict = errors.New("revision or identity conflict")
var ErrNotFound = errors.New("not found")

// ValidID validates user-facing Key, model, Context and binding names.
func ValidID(s string) bool { return naming.ValidID(s) }

// Instance/access IDs are base64url tokens and may start with an underscore or dash.
func ValidOpaqueID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

// Provider IDs follow the same user-facing naming rules.
func ValidProviderID(s string) bool { return ValidID(s) }

type Settings struct {
	InstanceID     string `json:"instance_id"`
	ControlAddress string `json:"control_address"`
	GatewayAddress string `json:"gateway_address"`
	Revision       int64  `json:"revision"`
}

func ValidateListen(s string) error {
	host, port, err := net.SplitHostPort(s)
	if err != nil || port == "" {
		return errors.New("listen address must be a numeric loopback IP and port")
	}
	n, portErr := strconv.Atoi(port)
	if portErr != nil || n < 1 || n > 65535 {
		return errors.New("listen port must be 1..65535")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("only numeric loopback listen addresses are permitted")
	}
	return nil
}

func ValidateURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid base URL; userinfo, query and fragment are prohibited")
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || ip == nil || !ip.IsLoopback() {
			return errors.New("upstreams require HTTPS (HTTP allowed only for a numeric loopback test endpoint)")
		}
	}
	return nil
}

const MaxProviderURLs = 16

// ValidateHomepage accepts an optional, display-only HTTP(S) page address.
func ValidateHomepage(s string) error {
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || len(s) > 2048 || strings.ContainsAny(s, " \t\r\n") {
		return errors.New("invalid homepage URL; use http(s) without userinfo")
	}
	return nil
}

// URLs returns the request base URLs, falling back to the legacy single field.
func (p Provider) URLs() []string {
	if len(p.BaseURLs) > 0 {
		return p.BaseURLs
	}
	if p.BaseURL != "" {
		return []string{p.BaseURL}
	}
	return nil
}

// Normalize makes BaseURLs authoritative and BaseURL its first entry.
func (p *Provider) Normalize() {
	p.BaseURLs = append([]string(nil), p.URLs()...)
	if len(p.BaseURLs) > 0 {
		p.BaseURL = p.BaseURLs[0]
	}
}

func ValidProtocol(p string) bool {
	return p == "responses" || p == "chat-completions" || p == "anthropic"
}

type Provider struct {
	ID          string `json:"id"`
	HomepageURL string `json:"homepage_url,omitempty"`
	// BaseURL mirrors BaseURLs[0] for older readers; BaseURLs is authoritative.
	BaseURL  string   `json:"base_url"`
	BaseURLs []string `json:"base_urls,omitempty"`
	// Protocol is legacy/internal: it only initializes a new default Key and
	// anonymous requests. Empty means "keep the stored value" on writes.
	Protocol       string `json:"protocol,omitempty"`
	Enabled        bool   `json:"enabled"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxConcurrent  int    `json:"max_concurrent"`
	Revision       int64  `json:"revision"`
	HasSecret      bool   `json:"has_secret"`
	Secret         string `json:"-"`
}

func (p Provider) Validate() error {
	if !ValidProviderID(p.ID) {
		return errors.New("invalid provider ID: use 1..128 Unicode letters (including Chinese), decimal digits, or ._-; start with a letter or digit")
	}
	urls := p.URLs()
	if len(urls) == 0 || len(urls) > MaxProviderURLs {
		return fmt.Errorf("provide 1..%d request base URLs", MaxProviderURLs)
	}
	seen := map[string]bool{}
	for _, u := range urls {
		if err := ValidateURL(u); err != nil {
			return err
		}
		key := strings.TrimRight(u, "/")
		if seen[key] {
			return errors.New("duplicate request base URL")
		}
		seen[key] = true
	}
	if p.BaseURL != "" && len(p.BaseURLs) > 0 && p.BaseURL != p.BaseURLs[0] {
		return errors.New("base_url must equal the first base_urls entry")
	}
	if err := ValidateHomepage(p.HomepageURL); err != nil {
		return err
	}
	if p.Protocol != "" && !ValidProtocol(p.Protocol) {
		return errors.New("unsupported protocol")
	}
	if p.TimeoutSeconds < 1 || p.TimeoutSeconds > 3600 || p.MaxConcurrent < 1 || p.MaxConcurrent > 64 {
		return errors.New("timeout must be 1..3600 seconds and concurrency 1..64")
	}
	if strings.ContainsAny(p.Secret, "\r\n") || len(p.Secret) > 8192 {
		return errors.New("invalid secret")
	}
	return nil
}

type Candidate struct {
	KeyIDs     []string `json:"key_ids,omitempty"`
	ProviderID string   `json:"provider_id"`
	Model      string   `json:"model"`
	Multiplier float64  `json:"multiplier,omitempty"`
}

func (c Candidate) EffectiveMultiplier() float64 {
	if c.Multiplier == 0 {
		return 1
	}
	return c.Multiplier
}

type Model struct {
	Strategy    string         `json:"strategy,omitempty"` // Empty preserves legacy ordered priority.
	ID          string         `json:"id"`
	DisplayName string         `json:"display_name"`
	Tools       bool           `json:"tools"`
	Streaming   bool           `json:"streaming"`
	Candidates  []Candidate    `json:"candidates"` // Ordered, explicit priority.
	Revision    int64          `json:"revision"`
	Codex       *CodexMetadata `json:"codex,omitempty"`
	Settings    *ModelSettings `json:"settings,omitempty"`
}

func (m Model) RoutingStrategy() string {
	if m.Strategy == "" {
		return "priority"
	}
	return m.Strategy
}

func ValidModelStrategy(v string) bool {
	return v == "" || v == "priority" || v == "round-robin" || v == "cache-priority" || v == "lowest-price" || v == "lowest-latency"
}

func (m Model) Validate() error {
	if !ValidModelStrategy(m.Strategy) {
		return errors.New("invalid model strategy")
	}
	if m.Settings != nil {
		if err := m.Settings.Validate(); err != nil {
			return err
		}
	}
	if m.Codex != nil {
		if err := m.Codex.Validate(); err != nil {
			return err
		}
	}
	if !ValidID(m.ID) || len(m.DisplayName) > 256 {
		return errors.New("invalid model ID or display name")
	}
	if len(m.Candidates) == 0 || len(m.Candidates) > 16 {
		return errors.New("a model requires 1..16 ordered candidates")
	}
	seen := map[string]bool{}
	for _, c := range m.Candidates {
		if len(c.KeyIDs) > 64 {
			return errors.New("too many candidate keys")
		}
		keyIDs := map[string]bool{}
		for _, id := range c.KeyIDs {
			if !ValidID(id) || keyIDs[id] {
				return errors.New("invalid candidate key")
			}
			keyIDs[id] = true
		}
		v := c.EffectiveMultiplier()
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0.01 || v > 1000 {
			return errors.New("candidate multiplier must be 0.01..1000")
		}
		if !ValidProviderID(c.ProviderID) || c.Model == "" || len(c.Model) > 256 || strings.ContainsAny(c.Model, "\r\n") {
			return errors.New("invalid model candidate")
		}
		key := c.ProviderID + "\x00" + c.Model
		if seen[key] {
			return errors.New("duplicate candidate")
		}
		seen[key] = true
	}
	return nil
}

type Context struct {
	SSHAuth           string `json:"ssh_auth,omitempty"`
	SSHHostKey        string `json:"ssh_host_key,omitempty"`
	Name              string `json:"name"`
	Transport         string `json:"transport,omitempty"`
	Endpoint          string `json:"endpoint,omitempty"`
	InstanceID        string `json:"expected_instance_id"`
	CredentialRef     string `json:"credential_ref"`
	SSHTarget         string `json:"ssh_target,omitempty"`
	SSHPort           int    `json:"ssh_port,omitempty"`
	SSHIdentityFile   string `json:"ssh_identity_file,omitempty"`
	RemoteControlPort int    `json:"remote_control_port,omitempty"`
	RemoteExecutable  string `json:"remote_executable,omitempty"`
	TrustNewHost      bool   `json:"trust_new_host,omitempty"`
	AccessID          string `json:"access_id,omitempty"`
}

func (c Context) Validate() error {
	if !ValidID(c.Name) || !ValidOpaqueID(c.InstanceID) {
		return errors.New("invalid context identity")
	}
	if c.Transport == "" || c.Transport == "local" {
		if c.CredentialRef != "local-owner" {
			return fmt.Errorf("invalid local context")
		}
		u, err := url.Parse(c.Endpoint)
		if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("invalid local management endpoint")
		}
		return ValidateListen(u.Host)
	}
	if (c.SSHAuth != "" && c.SSHAuth != "password") || len(c.SSHHostKey) > 8192 {
		return errors.New("invalid SSH authentication")
	}
	if c.Transport != "ssh" || c.CredentialRef != "ssh-manage" || !ValidOpaqueID(c.AccessID) {
		return errors.New("invalid SSH context")
	}
	if c.Endpoint != "" || c.SSHTarget == "" || strings.HasPrefix(c.SSHTarget, "-") || strings.ContainsAny(c.SSHTarget, "\x00\r\n\t ") {
		return errors.New("invalid SSH target")
	}
	for _, r := range c.SSHTarget {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && !strings.ContainsRune("_.@:%[]-", r) {
			return errors.New("invalid SSH target")
		}
	}
	if c.SSHPort < 0 || c.SSHPort > 65535 || c.RemoteControlPort < 1 || c.RemoteControlPort > 65535 {
		return errors.New("invalid SSH port")
	}
	if strings.ContainsAny(c.SSHIdentityFile, "\x00\r\n") || c.RemoteExecutable == "" || strings.HasPrefix(c.RemoteExecutable, "-") || len(c.RemoteExecutable) > 512 {
		return errors.New("invalid SSH option")
	}
	for _, r := range c.RemoteExecutable {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && !strings.ContainsRune("_./~:-", r) {
			return errors.New("invalid remote executable")
		}
	}
	return nil
}

type Access struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Scope     string `json:"scope"`
	ExpiresAt int64  `json:"expires_at"`
	Revoked   bool   `json:"revoked"`
}
