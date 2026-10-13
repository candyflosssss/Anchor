package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/candyflosssss/Anchor/internal/providerapi"
)

// ProviderPlatform is an internal credential-bearing read. Config.UserToken
// is excluded from JSON; public readers must use ProviderPlatformSettings.
func (s *Store) ProviderPlatform(id string) (providerapi.Config, error) {
	var document, secret string
	err := s.db.QueryRow(`SELECT document,user_token FROM provider_platforms WHERE provider_id=?`, id).Scan(&document, &secret)
	if errors.Is(err, sql.ErrNoRows) {
		return providerapi.Config{}, nil
	}
	if err != nil {
		return providerapi.Config{}, err
	}
	var c providerapi.Config
	if json.Unmarshal([]byte(document), &c) != nil {
		return c, ErrInvalid
	}
	c.UserToken = secret
	c.HasUserToken = secret != ""
	return c, nil
}
func (s *Store) ProviderPlatformSettings(id string) (providerapi.Settings, error) {
	// Read the document, credential presence and provider revision in one query.
	var rev int64
	var doc sql.NullString
	var present bool
	err := s.db.QueryRow(`SELECT p.revision,c.document,COALESCE(length(c.user_token)>0,0) FROM providers p LEFT JOIN provider_platforms c ON c.provider_id=p.id WHERE p.id=?`, id).Scan(&rev, &doc, &present)
	if errors.Is(err, sql.ErrNoRows) {
		return providerapi.Settings{}, ErrNotFound
	}
	if err != nil {
		return providerapi.Settings{}, err
	}
	out := providerapi.Settings{ProviderID: id, Revision: rev}
	if doc.Valid && json.Unmarshal([]byte(doc.String), &out.Config) != nil {
		return out, ErrInvalid
	}
	out.Config.HasUserToken = present
	return out, nil
}
func (s *Store) ConfigureProviderPlatform(in providerapi.Configure) (providerapi.Settings, error) {
	if !ValidProviderID(in.ProviderID) || in.ExpectedRevision < 1 || in.Config.Platform == "" {
		return providerapi.Settings{}, ErrInvalid
	}
	c := in.Config
	c.SiteURL = strings.TrimRight(c.SiteURL, "/")
	c.UserToken = ""
	if in.UserToken != nil {
		c.UserToken = *in.UserToken
	}
	if c.Validate() != nil {
		return providerapi.Settings{}, ErrInvalid
	}
	if c.SiteURL != "" && ValidateURL(c.SiteURL) != nil {
		return providerapi.Settings{}, ErrInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return providerapi.Settings{}, err
	}
	defer tx.Rollback()
	var rev int64
	var pdoc string
	err = tx.QueryRow(`SELECT revision,document FROM providers WHERE id=?`, in.ProviderID).Scan(&rev, &pdoc)
	if errors.Is(err, sql.ErrNoRows) {
		return providerapi.Settings{}, ErrNotFound
	}
	if err != nil {
		return providerapi.Settings{}, err
	}
	if rev != in.ExpectedRevision {
		return providerapi.Settings{}, ErrConflict
	}
	var oldDoc, secret string
	err = tx.QueryRow(`SELECT document,user_token FROM provider_platforms WHERE provider_id=?`, in.ProviderID).Scan(&oldDoc, &secret)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return providerapi.Settings{}, err
	}
	var old providerapi.Config
	if oldDoc != "" && json.Unmarshal([]byte(oldDoc), &old) != nil {
		return providerapi.Settings{}, ErrInvalid
	}
	if in.UserToken == nil {
		// Changing an authorization target never silently forwards its old token.
		if old.Platform != c.Platform || old.SiteURL != c.SiteURL || old.UserID != c.UserID {
			secret = ""
		}
		c.UserToken = secret
	}
	c.HasUserToken = c.UserToken != ""
	b, err := json.Marshal(c)
	if err != nil {
		return providerapi.Settings{}, err
	}
	if _, err = tx.Exec(`INSERT INTO provider_platforms(provider_id,document,user_token) VALUES(?,?,?) ON CONFLICT(provider_id) DO UPDATE SET document=excluded.document,user_token=excluded.user_token`, in.ProviderID, string(b), c.UserToken); err != nil {
		return providerapi.Settings{}, err
	}
	var p Provider
	if json.Unmarshal([]byte(pdoc), &p) != nil {
		return providerapi.Settings{}, ErrInvalid
	}
	p.Revision = rev + 1
	b, _ = json.Marshal(p)
	if _, err = tx.Exec(`UPDATE providers SET revision=?,document=? WHERE id=?`, p.Revision, string(b), p.ID); err != nil {
		return providerapi.Settings{}, err
	}
	if err = tx.Commit(); err != nil {
		return providerapi.Settings{}, err
	}
	c.UserToken = ""
	return providerapi.Settings{ProviderID: in.ProviderID, Revision: rev + 1, Config: c}, nil
}
