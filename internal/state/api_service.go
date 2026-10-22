package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// APIServiceSettings controls the optional application listener; managed Agents
// keep their existing loopback gateway and credentials.
type APIServiceSettings struct {
	Enabled  bool  `json:"enabled"`
	Port     int   `json:"port"`
	AllowLAN bool  `json:"allow_lan"`
	Revision int64 `json:"revision"`
}
type SaveAPIServiceSettings struct {
	Enabled          *bool `json:"enabled"`
	Port             int   `json:"port"`
	AllowLAN         *bool `json:"allow_lan"`
	ExpectedRevision int64 `json:"expected_revision"`
}
type IssueAPIKey struct {
	Label      string `json:"label"`
	TTLSeconds int64  `json:"ttl_seconds"`
}
type IssuedAPIKey struct {
	Access Access `json:"access"`
	Token  string `json:"token"`
}

func (in IssueAPIKey) Validate() error {
	if strings.TrimSpace(in.Label) == "" || len(in.Label) > 128 || strings.IndexFunc(in.Label, unicode.IsControl) >= 0 || in.TTLSeconds < 0 || in.TTLSeconds > 315360000 {
		return ErrInvalid
	}
	return nil
}
func (in SaveAPIServiceSettings) Validate() error {
	if in.Enabled == nil || in.AllowLAN == nil || in.Port < 1 || in.Port > 65535 || in.ExpectedRevision < 1 || in.ExpectedRevision >= 9007199254740991 {
		return ErrInvalid
	}
	return nil
}
func (v APIServiceSettings) Address() string {
	host := "127.0.0.1"
	if v.AllowLAN {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, strconv.Itoa(v.Port))
}
func readAPIServiceSettings(q interface{ QueryRow(string, ...any) *sql.Row }) (APIServiceSettings, error) {
	out := APIServiceSettings{Port: 17862, Revision: 1}
	var raw string
	err := q.QueryRow(`SELECT value FROM meta WHERE key='api_service_settings'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if json.Unmarshal([]byte(raw), &out) != nil {
		return out, ErrInvalid
	}
	if (SaveAPIServiceSettings{Enabled: &out.Enabled, Port: out.Port, AllowLAN: &out.AllowLAN, ExpectedRevision: min(out.Revision, 9007199254740990)}).Validate() != nil || out.Revision > 9007199254740991 {
		return out, ErrInvalid
	}
	return out, nil
}
func (s *Store) APIServiceSettings() (APIServiceSettings, error) { return readAPIServiceSettings(s.db) }
func (s *Store) SaveAPIServiceSettings(in SaveAPIServiceSettings) (APIServiceSettings, error) {
	if err := in.Validate(); err != nil {
		return APIServiceSettings{}, err
	}
	cfg, err := s.Settings()
	if err != nil {
		return APIServiceSettings{}, err
	}
	for _, address := range []string{cfg.ControlAddress, cfg.GatewayAddress} {
		_, port, _ := net.SplitHostPort(address)
		number, _ := strconv.Atoi(port)
		if number == in.Port {
			return APIServiceSettings{}, ErrInvalid
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return APIServiceSettings{}, err
	}
	defer tx.Rollback()
	old, err := readAPIServiceSettings(tx)
	if err != nil {
		return old, err
	}
	if old.Revision != in.ExpectedRevision {
		return old, ErrConflict
	}
	out := APIServiceSettings{Enabled: *in.Enabled, Port: in.Port, AllowLAN: *in.AllowLAN, Revision: old.Revision + 1}
	raw, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(`INSERT INTO meta(key,value) VALUES('api_service_settings',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw))
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// AccessForToken returns only verified public identity, never a hash or secret.
func (s *Store) AccessForToken(token, scope string) (Access, error) {
	var a Access
	if token == "" || len(token) > 8192 {
		return a, ErrNotFound
	}
	err := s.db.QueryRow(`SELECT id,label,scope,expires_at,revoked FROM access WHERE hash=? AND scope=? AND revoked=0 AND (expires_at=0 OR expires_at>?)`, digest(token), scope, time.Now().Unix()).Scan(&a.ID, &a.Label, &a.Scope, &a.ExpiresAt, &a.Revoked)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return a, err
}
func (s *Store) GatewayKeys() ([]Access, error) {
	all, err := s.AccessList()
	if err != nil {
		return nil, err
	}
	out := []Access{}
	for _, a := range all {
		if a.Scope == "gateway" {
			out = append(out, a)
		}
	}
	return out, nil
}
func (s *Store) RevokeGatewayKey(id string) error {
	res, err := s.db.Exec(`UPDATE access SET revoked=1 WHERE id=? AND scope='gateway'`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type APIServiceWorkspace struct {
	Settings        APIServiceSettings `json:"settings"`
	ActiveAddress   string             `json:"active_address"`
	RestartRequired bool               `json:"restart_required"`
	BaseURLs        []string           `json:"base_urls"`
	Keys            []Access           `json:"keys"`
}
