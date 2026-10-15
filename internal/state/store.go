package state

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/candyflosssss/Anchor/internal/platform"
	_ "modernc.org/sqlite"
)

type Store struct {
	db  *sql.DB
	Dir string
}

func DefaultDir() (string, error) { d, e := os.UserConfigDir(); return filepath.Join(d, "anchor"), e }
func Token() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func digest(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func Open(dir string, create bool) (*Store, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "state.db")
	if !create {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("instance not initialized; run anchor init: %w", err)
		}
	}
	if err := platform.SecureDir(dir); err != nil {
		return nil, err
	}
	if err := platform.SecureFile(path); err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	dsn := (&url.URL{Scheme: "file", Path: uriPath, RawQuery: "_txlock=immediate"}).String()
	// IMMEDIATE prevents a read-then-write transaction from racing another writer.
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, Dir: dir}
	fail := func(err error) (*Store, error) { db.Close(); return nil, err }
	if _, err = db.Exec(`PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL;`); err != nil {
		return fail(err)
	}
	var version int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fail(err)
	}
	if version > 8 {
		return fail(errors.New("state schema is newer than this binary"))
	}
	if version == 0 {
		if !create {
			return fail(errors.New("uninitialized state schema; run anchor init"))
		}
		_, err = db.Exec(`BEGIN IMMEDIATE;
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE providers (id TEXT PRIMARY KEY, document TEXT NOT NULL, secret TEXT NOT NULL, revision INTEGER NOT NULL);
CREATE TABLE models (id TEXT PRIMARY KEY, document TEXT NOT NULL, revision INTEGER NOT NULL);
CREATE TABLE access (id TEXT PRIMARY KEY, label TEXT NOT NULL, scope TEXT NOT NULL, hash TEXT UNIQUE NOT NULL, expires_at INTEGER NOT NULL, revoked INTEGER NOT NULL DEFAULT 0);
CREATE TABLE contexts (name TEXT PRIMARY KEY, document TEXT NOT NULL);
PRAGMA user_version=1;
COMMIT;`)
		if err != nil {
			return fail(err)
		}
	}
	if version < 2 {
		_, err = db.Exec(`BEGIN IMMEDIATE;
CREATE TABLE bindings (id TEXT PRIMARY KEY, root TEXT UNIQUE NOT NULL, document TEXT NOT NULL, token TEXT NOT NULL, hash TEXT UNIQUE NOT NULL, enabled INTEGER NOT NULL, revision INTEGER NOT NULL);
PRAGMA user_version=2;
COMMIT;`)
		if err != nil {
			return fail(err)
		}
	}
	if version < 3 {
		_, err = db.Exec(`BEGIN IMMEDIATE;
CREATE TABLE provider_keys (provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE ON UPDATE CASCADE, id TEXT NOT NULL, secret TEXT NOT NULL, enabled INTEGER NOT NULL, PRIMARY KEY(provider_id,id));
INSERT INTO provider_keys SELECT id,'default',secret,1 FROM providers WHERE secret!='';
CREATE TABLE provider_strategy (provider_id TEXT PRIMARY KEY REFERENCES providers(id) ON DELETE CASCADE ON UPDATE CASCADE, strategy TEXT NOT NULL);
CREATE TABLE provider_prices (provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE ON UPDATE CASCADE, model TEXT NOT NULL, document TEXT NOT NULL, PRIMARY KEY(provider_id,model));
CREATE TABLE usage_totals (provider_id TEXT NOT NULL, model TEXT NOT NULL, requests INTEGER NOT NULL, failures INTEGER NOT NULL, elapsed_ms INTEGER NOT NULL, known INTEGER NOT NULL, input_tokens INTEGER NOT NULL, output_tokens INTEGER NOT NULL, cache_read INTEGER NOT NULL, cache_write INTEGER NOT NULL, PRIMARY KEY(provider_id,model));
PRAGMA user_version=3;
COMMIT;`)
		if err != nil {
			return fail(err)
		}
	}
	if version < 4 {
		_, err = db.Exec(`BEGIN IMMEDIATE;
ALTER TABLE provider_keys ADD COLUMN protocol TEXT NOT NULL DEFAULT 'responses';
UPDATE provider_keys SET protocol=(SELECT json_extract(document,'$.protocol') FROM providers WHERE providers.id=provider_keys.provider_id);
CREATE TABLE key_model_protocols (
 provider_id TEXT NOT NULL, key_id TEXT NOT NULL, model TEXT NOT NULL, protocol TEXT NOT NULL,
 PRIMARY KEY(provider_id,key_id,model),
 FOREIGN KEY(provider_id,key_id) REFERENCES provider_keys(provider_id,id) ON DELETE CASCADE ON UPDATE CASCADE
);
PRAGMA user_version=4;
COMMIT;`)
		if err != nil {
			return fail(err)
		}
	}

	if version < 5 {
		_, err = db.Exec(`BEGIN IMMEDIATE;
CREATE TABLE provider_platforms (
 provider_id TEXT PRIMARY KEY REFERENCES providers(id) ON DELETE CASCADE ON UPDATE CASCADE,
 document TEXT NOT NULL, user_token TEXT NOT NULL
);
PRAGMA user_version=5;
COMMIT;`)
		if err != nil {
			return fail(err)
		}
	}

	// Candidate Key restrictions must never be silently ignored by older cores.
	if version < 6 {
		if _, err = db.Exec(`PRAGMA user_version=6`); err != nil {
			return fail(err)
		}
	}
	if version < 7 {
		_, err = db.Exec(`BEGIN IMMEDIATE;
CREATE TABLE IF NOT EXISTS context_credentials (
 name TEXT PRIMARY KEY REFERENCES contexts(name) ON DELETE CASCADE,
 secret TEXT NOT NULL
);
PRAGMA user_version=7;
COMMIT;`)
		if err != nil {
			return fail(err)
		}
	}
	if version < 8 {
		_, err = db.Exec(`BEGIN IMMEDIATE;
CREATE TABLE IF NOT EXISTS key_rate_cache (
 provider_id TEXT NOT NULL, key_id TEXT NOT NULL, document TEXT NOT NULL,
 PRIMARY KEY(provider_id,key_id),
 FOREIGN KEY(provider_id,key_id) REFERENCES provider_keys(provider_id,id) ON DELETE CASCADE ON UPDATE CASCADE
);
PRAGMA user_version=8;
COMMIT;`)
		if err != nil {
			return fail(err)
		}
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) OwnerToken() (string, error) {
	path := filepath.Join(s.Dir, "owner.token")
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	if err := platform.SecureFile(path); err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(b))
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return "", errors.New("invalid owner credential file")
	}
	return token, nil
}
func (s *Store) Settings() (Settings, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key='settings'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{}, ErrNotFound
	}
	if err != nil {
		return Settings{}, err
	}
	var out Settings
	err = json.Unmarshal([]byte(v), &out)
	return out, err
}

// Initialize is called under the instance lock. Re-running it never rotates identity.
func (s *Store) Initialize(control, gateway string) (Settings, error) {
	if existing, err := s.Settings(); err == nil {
		if _, err := s.OwnerToken(); err != nil {
			return Settings{}, err
		}
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Settings{}, err
	}
	if err := ValidateListen(control); err != nil {
		return Settings{}, err
	}
	if err := ValidateListen(gateway); err != nil {
		return Settings{}, err
	}
	if control == gateway {
		return Settings{}, errors.New("management and gateway listeners must be distinct")
	}
	token, err := s.OwnerToken()
	if errors.Is(err, os.ErrNotExist) {
		token, err = Token()
		if err != nil {
			return Settings{}, err
		}
		path := filepath.Join(s.Dir, "owner.token")
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return Settings{}, err
		}
		if err = platform.SecureFile(path); err == nil {
			_, err = f.WriteString(token + "\n")
		}
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return Settings{}, err
		}
	} else if err != nil {
		return Settings{}, err
	}
	id, err := Token()
	if err != nil {
		return Settings{}, err
	}
	settings := Settings{InstanceID: id, ControlAddress: control, GatewayAddress: gateway, Revision: 1}
	b, _ := json.Marshal(settings)
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return Settings{}, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO meta(key,value) VALUES ('settings',?),('owner_hash',?)`, string(b), digest(token)); err != nil {
		return Settings{}, err
	}
	if err = tx.Commit(); err != nil {
		return Settings{}, err
	}
	return settings, nil
}
func (s *Store) Authenticate(token, scope string) bool {
	if token == "" || len(token) > 8192 {
		return false
	}
	hash := digest(token)
	if scope == "manage" {
		var owner string
		if err := s.db.QueryRow(`SELECT value FROM meta WHERE key='owner_hash'`).Scan(&owner); err == nil && hash == owner {
			return true
		}
	}
	var count int
	err := s.db.QueryRow(`SELECT count(*) FROM access WHERE hash=? AND scope=? AND revoked=0 AND (expires_at=0 OR expires_at>?)`, hash, scope, time.Now().Unix()).Scan(&count)
	return err == nil && count == 1
}
func (s *Store) Grant(label, scope string, ttl time.Duration) (Access, string, error) {
	if label == "" || len(label) > 128 || strings.ContainsAny(label, "\r\n") || (scope != "manage" && scope != "gateway") || ttl < 0 {
		return Access{}, "", errors.New("invalid label, scope or expiry")
	}
	if _, err := s.Settings(); err != nil {
		return Access{}, "", err
	}
	id, err := Token()
	if err != nil {
		return Access{}, "", err
	}
	token, err := Token()
	if err != nil {
		return Access{}, "", err
	}
	a := Access{ID: id, Label: label, Scope: scope}
	if ttl > 0 {
		a.ExpiresAt = time.Now().Add(ttl).Unix()
	}
	_, err = s.db.Exec(`INSERT INTO access(id,label,scope,hash,expires_at) VALUES(?,?,?,?,?)`, a.ID, a.Label, a.Scope, digest(token), a.ExpiresAt)
	return a, token, err
}
func (s *Store) AccessList() ([]Access, error) {
	rows, err := s.db.Query(`SELECT id,label,scope,expires_at,revoked FROM access ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Access{}
	for rows.Next() {
		var a Access
		if err := rows.Scan(&a.ID, &a.Label, &a.Scope, &a.ExpiresAt, &a.Revoked); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Store) Revoke(id string) error {
	res, err := s.db.Exec(`UPDATE access SET revoked=1 WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) Providers() ([]Provider, error) {
	rows, err := s.db.Query(`SELECT document,secret,revision FROM providers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Provider{}
	for rows.Next() {
		var b, secret string
		var rev int64
		if err := rows.Scan(&b, &secret, &rev); err != nil {
			return nil, err
		}
		var p Provider
		if err = json.Unmarshal([]byte(b), &p); err != nil {
			return nil, err
		}
		p.Normalize()
		p.Revision = rev
		p.HasSecret = secret != ""
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) Provider(id string) (Provider, error) {
	var b, secret string
	var rev int64
	err := s.db.QueryRow(`SELECT document,secret,revision FROM providers WHERE id=?`, id).Scan(&b, &secret, &rev)
	if errors.Is(err, sql.ErrNoRows) {
		return Provider{}, ErrNotFound
	}
	if err != nil {
		return Provider{}, err
	}
	var p Provider
	if err = json.Unmarshal([]byte(b), &p); err != nil {
		return Provider{}, err
	}
	p.Normalize()
	p.Secret = secret
	p.HasSecret = secret != ""
	p.Revision = rev
	return p, nil
}

// PutProvider uses optimistic concurrency; expected=0 means create only.
// nil secret preserves an existing credential; an explicit empty secret removes it.
func (s *Store) PutProvider(p Provider, expected int64, secret *string) (Provider, error) {
	if secret != nil {
		p.Secret = *secret
	}
	if err := p.Validate(); err != nil {
		return Provider{}, err
	}
	if expected < 0 {
		return Provider{}, ErrConflict
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Provider{}, err
	}
	defer tx.Rollback()
	var oldSecret, oldDocument string
	var current int64
	err = tx.QueryRow(`SELECT secret,revision,document FROM providers WHERE id=?`, p.ID).Scan(&oldSecret, &current, &oldDocument)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Provider{}, err
	}
	if current != expected {
		return Provider{}, ErrConflict
	}
	if p.CostMultiplier == 0 && oldDocument != "" {
		var old Provider
		if json.Unmarshal([]byte(oldDocument), &old) == nil {
			p.CostMultiplier = old.CostMultiplier
		}
	}
	p.Normalize()
	if p.Protocol == "" {
		// Provider-level protocol is no longer user-facing; keep the stored legacy value.
		var old Provider
		if oldDocument != "" && json.Unmarshal([]byte(oldDocument), &old) == nil && ValidProtocol(old.Protocol) {
			p.Protocol = old.Protocol
		} else {
			p.Protocol = "responses"
		}
	}
	if secret == nil {
		p.Secret = oldSecret
	}
	p.Revision = expected + 1
	p.HasSecret = p.Secret != ""
	b, _ := json.Marshal(p)
	if current == 0 {
		_, err = tx.Exec(`INSERT INTO providers(id,document,secret,revision) VALUES(?,?,?,?)`, p.ID, string(b), p.Secret, p.Revision)
	} else {
		var result sql.Result
		result, err = tx.Exec(`UPDATE providers SET document=?,secret=?,revision=? WHERE id=? AND revision=?`, string(b), p.Secret, p.Revision, p.ID, expected)
		if err == nil {
			n, _ := result.RowsAffected()
			if n != 1 {
				return Provider{}, ErrConflict
			}
		}
	}
	if err != nil {
		return Provider{}, err
	}
	// A derived dashboard URL follows BaseURL. Never carry a dashboard token
	// across a request-root change; an explicit SiteURL remains independent.
	if oldDocument != "" {
		var old Provider
		if json.Unmarshal([]byte(oldDocument), &old) == nil && old.BaseURL != p.BaseURL {
			if _, err = tx.Exec(`UPDATE provider_platforms SET user_token='' WHERE provider_id=? AND COALESCE(json_extract(document,'$.site_url'),'')=''`, p.ID); err != nil {
				return Provider{}, err
			}
		}
	}
	if oldDocument != "" {
		var old Provider
		if json.Unmarshal([]byte(oldDocument), &old) == nil && strings.Join(old.URLs(), "\n") != strings.Join(p.URLs(), "\n") {
			if _, err = tx.Exec(`DELETE FROM key_rate_cache WHERE provider_id=?`, p.ID); err != nil {
				return Provider{}, err
			}
		}
	}
	if secret != nil {
		if _, err = tx.Exec(`DELETE FROM key_rate_cache WHERE provider_id=? AND key_id='default' AND EXISTS(SELECT 1 FROM provider_keys WHERE provider_id=? AND id='default' AND secret!=?)`, p.ID, p.ID, *secret); err != nil {
			return Provider{}, err
		}
		if *secret == "" {
			if err = checkKeyUnreferenced(tx, p.ID, "default"); err != nil {
				return Provider{}, err
			}
			_, err = tx.Exec(`DELETE FROM provider_keys WHERE provider_id=? AND id='default'`, p.ID)
		} else {
			_, err = tx.Exec(`INSERT INTO provider_keys(provider_id,id,secret,enabled,protocol) VALUES(?,'default',?,1,?) ON CONFLICT(provider_id,id) DO UPDATE SET secret=excluded.secret,enabled=1`, p.ID, *secret, p.Protocol)
		}
		if err != nil {
			return Provider{}, err
		}
	}
	var keyCount int
	if err = tx.QueryRow(`SELECT count(*) FROM provider_keys WHERE provider_id=?`, p.ID).Scan(&keyCount); err != nil {
		return Provider{}, err
	}
	p.HasSecret = keyCount > 0
	b, _ = json.Marshal(p)
	if _, err = tx.Exec(`UPDATE providers SET document=? WHERE id=?`, string(b), p.ID); err != nil {
		return Provider{}, err
	}
	if err = tx.Commit(); err != nil {
		return Provider{}, err
	}
	p.Secret = ""
	return p, nil
}
func (s *Store) Models() ([]Model, error) {
	rows, err := s.db.Query(`SELECT document,revision FROM models ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Model{}
	for rows.Next() {
		var b string
		var rev int64
		if err := rows.Scan(&b, &rev); err != nil {
			return nil, err
		}
		var m Model
		if err = json.Unmarshal([]byte(b), &m); err != nil {
			return nil, err
		}
		m.Revision = rev
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Store) Model(id string) (Model, error) {
	var b string
	var rev int64
	err := s.db.QueryRow(`SELECT document,revision FROM models WHERE id=?`, id).Scan(&b, &rev)
	if errors.Is(err, sql.ErrNoRows) {
		return Model{}, ErrNotFound
	}
	if err != nil {
		return Model{}, err
	}
	var m Model
	err = json.Unmarshal([]byte(b), &m)
	m.Revision = rev
	return m, err
}
func (s *Store) PutModel(m Model, expected int64) (Model, error) {
	return s.PutProviderModel(m, expected, "", 0)
}
func (s *Store) PutProviderModel(m Model, expected int64, providerID string, providerRevision int64) (Model, error) {
	if err := m.Validate(); err != nil {
		return Model{}, err
	}
	if expected < 0 {
		return Model{}, ErrConflict
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Model{}, err
	}
	defer tx.Rollback()
	if providerID != "" {
		var rev int64
		if err = tx.QueryRow(`SELECT revision FROM providers WHERE id=?`, providerID).Scan(&rev); err != nil {
			return Model{}, err
		}
		if rev != providerRevision {
			return Model{}, ErrConflict
		}
	}
	var current int64
	err = tx.QueryRow(`SELECT revision FROM models WHERE id=?`, m.ID).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Model{}, err
	}
	if current != expected {
		return Model{}, ErrConflict
	}
	for _, c := range m.Candidates {
		var n int
		if err = tx.QueryRow(`SELECT count(*) FROM providers WHERE id=?`, c.ProviderID).Scan(&n); err != nil {
			return Model{}, err
		}
		if n != 1 {
			return Model{}, fmt.Errorf("candidate provider does not exist: %s", c.ProviderID)
		}
		for _, id := range c.KeyIDs {
			if err = tx.QueryRow(`SELECT count(*) FROM provider_keys WHERE provider_id=? AND id=?`, c.ProviderID, id).Scan(&n); err != nil {
				return Model{}, err
			}
			if n == 1 {
				continue
			}
			if id != "anonymous" {
				return Model{}, ErrInvalid
			}
			if err = tx.QueryRow(`SELECT count(*) FROM provider_keys WHERE provider_id=?`, c.ProviderID).Scan(&n); err != nil {
				return Model{}, err
			}
			if n != 0 {
				return Model{}, ErrInvalid
			}
		}
	}
	m.Revision = expected + 1
	b, _ := json.Marshal(m)
	if current == 0 {
		_, err = tx.Exec(`INSERT INTO models(id,document,revision) VALUES(?,?,?)`, m.ID, string(b), m.Revision)
	} else {
		var res sql.Result
		res, err = tx.Exec(`UPDATE models SET document=?,revision=? WHERE id=? AND revision=?`, string(b), m.Revision, m.ID, expected)
		if err == nil {
			n, _ := res.RowsAffected()
			if n != 1 {
				return Model{}, ErrConflict
			}
		}
	}
	if err != nil {
		return Model{}, err
	}
	if err = tx.Commit(); err != nil {
		return Model{}, err
	}
	return m, nil
}
func (s *Store) PutContext(c Context) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, _ := json.Marshal(c)
	_, err := s.db.Exec(`INSERT INTO contexts(name,document) VALUES(?,?)`, c.Name, string(b))
	return err
}
func (s *Store) PutContextWithCredential(c Context, secret string) error {
	if err := c.Validate(); err != nil || c.Transport != "ssh" || secret == "" || len(secret) > 8192 {
		if err != nil {
			return err
		}
		return errors.New("invalid context credential")
	}
	b, _ := json.Marshal(c)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO contexts(name,document) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET document=excluded.document`, c.Name, string(b)); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO context_credentials(name,secret) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET secret=excluded.secret`, c.Name, secret); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ContextCredential(name string) (string, error) {
	var secret string
	err := s.db.QueryRow(`SELECT secret FROM context_credentials WHERE name=?`, name).Scan(&secret)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return secret, err
}

// DesktopSelection is separate from the CLI's current_context preference.
func (s *Store) DesktopSelection() (string, error) {
	var name string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key='desktop_context'`).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "@local", nil
	}
	return name, err
}
func (s *Store) SelectDesktop(name string) error {
	if name != "@local" {
		c, err := s.Context(name)
		if err != nil {
			return err
		}
		if c.Transport != "ssh" {
			return ErrInvalid
		}
	}
	_, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES('desktop_context',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, name)
	return err
}
func (s *Store) Context(name string) (Context, error) {
	if name == "" {
		if err := s.db.QueryRow(`SELECT value FROM meta WHERE key='current_context'`).Scan(&name); err != nil {
			return Context{}, errors.New("no current context; add and select a local context")
		}
	}
	var b string
	err := s.db.QueryRow(`SELECT document FROM contexts WHERE name=?`, name).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return Context{}, ErrNotFound
	}
	if err != nil {
		return Context{}, err
	}
	var c Context
	err = json.Unmarshal([]byte(b), &c)
	return c, err
}
func (s *Store) Contexts() ([]Context, error) {
	rows, err := s.db.Query(`SELECT document FROM contexts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Context{}
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var c Context
		if err := json.Unmarshal([]byte(b), &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) UseContext(name string) error {
	if _, err := s.Context(name); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES('current_context',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, name)
	return err
}

// A source Key cannot be deleted while a mapping explicitly references it.
func checkKeyUnreferenced(tx *sql.Tx, provider, key string) error {
	var n int
	err := tx.QueryRow(`SELECT count(*) FROM models m, json_each(m.document,'$.candidates') c, json_each(c.value,'$.key_ids') k WHERE json_extract(c.value,'$.provider_id')=? AND k.value=?`, provider, key).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrReferenced
	}
	return nil
}
