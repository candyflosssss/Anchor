package state

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrConflict = errors.New("revision or identity conflict")
var ErrNotFound = errors.New("not found")
var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

func ValidID(s string) bool { return identifier.MatchString(s) }

// Provider IDs are user-facing labels, not filesystem names or model slugs.
// Keep the ASCII-only ValidID contract for contexts, models and bindings.
var providerIdentifier = regexp.MustCompile(`^[\p{L}\p{Nd}][\p{L}\p{Nd}._-]{0,127}$`)

func ValidProviderID(s string) bool {
	return utf8.ValidString(s) && providerIdentifier.MatchString(s)
}

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

type Provider struct {
	ID             string `json:"id"`
	BaseURL        string `json:"base_url"`
	Protocol       string `json:"protocol"`
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
	if err := ValidateURL(p.BaseURL); err != nil {
		return err
	}
	if p.Protocol != "responses" {
		return errors.New("unsupported protocol: this milestone supports only responses")
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
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}
type Model struct {
	ID          string         `json:"id"`
	DisplayName string         `json:"display_name"`
	Tools       bool           `json:"tools"`
	Streaming   bool           `json:"streaming"`
	Candidates  []Candidate    `json:"candidates"` // Ordered, explicit priority.
	Revision    int64          `json:"revision"`
	Codex       *CodexMetadata `json:"codex,omitempty"`
}

func (m Model) Validate() error {
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
	Name          string `json:"name"`
	Endpoint      string `json:"endpoint"`
	InstanceID    string `json:"expected_instance_id"`
	CredentialRef string `json:"credential_ref"`
}

func (c Context) Validate() error {
	if !ValidID(c.Name) || c.InstanceID == "" || c.CredentialRef != "local-owner" {
		return fmt.Errorf("invalid local context")
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid local management endpoint")
	}
	return ValidateListen(u.Host)
}

type Access struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Scope     string `json:"scope"`
	ExpiresAt int64  `json:"expires_at"`
	Revoked   bool   `json:"revoked"`
}
