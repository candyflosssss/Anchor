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
	if version > 1 {
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
	var oldSecret string
	var current int64
	err = tx.QueryRow(`SELECT secret,revision FROM providers WHERE id=?`, p.ID).Scan(&oldSecret, &current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Provider{}, err
	}
	if current != expected {
		return Provider{}, ErrConflict
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
