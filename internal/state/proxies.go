package state

import (
	"database/sql"
	"errors"
	"strings"
)

// Public proxy DTOs deliberately never contain a URL, credential or node configuration.
type ProxyProfile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Revision int64  `json:"revision"`
}
type ProxyBinding struct {
	ProviderID string `json:"provider_id"`
	KeyID      string `json:"key_id"`
	Mode       string `json:"mode"`
	ProxyID    string `json:"proxy_id"`
}
type ProxyTarget struct {
	ProviderID string `json:"provider_id"`
	KeyID      string `json:"key_id"`
}
type ProxyEngine struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
	Platform  string `json:"platform"`
	Running   int    `json:"running"`
}
type ProxyWorkspace struct {
	Revision int64          `json:"revision"`
	Profiles []ProxyProfile `json:"profiles"`
	Bindings []ProxyBinding `json:"bindings"`
	Targets  []ProxyTarget  `json:"targets"`
	Engine   ProxyEngine    `json:"engine"`
}
type ProxyAction struct {
	Action           string       `json:"action"`
	ExpectedRevision int64        `json:"expected_revision"`
	Approved         bool         `json:"approved"`
	Profile          ProxyProfile `json:"profile"`
	Secret           *string      `json:"secret"`
	NodeName         string       `json:"node_name"`
	Binding          ProxyBinding `json:"binding"`
}
type ProxySecret struct {
	Profile ProxyProfile
	Secret  string `json:"-"`
}

func (s *Store) ProxyWorkspace() (ProxyWorkspace, error) {
	out := ProxyWorkspace{Profiles: []ProxyProfile{}, Bindings: []ProxyBinding{}, Targets: []ProxyTarget{}}
	tx, e := s.db.Begin()
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	if e = tx.QueryRow(`SELECT revision FROM proxy_revision`).Scan(&out.Revision); e != nil {
		return out, e
	}
	rows, e := tx.Query(`SELECT id,name,kind,revision FROM proxy_profiles ORDER BY id`)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var p ProxyProfile
		if e = rows.Scan(&p.ID, &p.Name, &p.Kind, &p.Revision); e != nil {
			rows.Close()
			return out, e
		}
		out.Profiles = append(out.Profiles, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.Query(`SELECT provider_id,'',mode,COALESCE(proxy_id,'') FROM provider_proxy UNION ALL SELECT provider_id,key_id,mode,COALESCE(proxy_id,'') FROM key_proxy ORDER BY 1,2`)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var b ProxyBinding
		if e = rows.Scan(&b.ProviderID, &b.KeyID, &b.Mode, &b.ProxyID); e != nil {
			rows.Close()
			return out, e
		}
		out.Bindings = append(out.Bindings, b)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.Query(`SELECT id,'' FROM providers UNION ALL SELECT provider_id,id FROM provider_keys ORDER BY 1,2`)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var t ProxyTarget
		if e = rows.Scan(&t.ProviderID, &t.KeyID); e != nil {
			rows.Close()
			return out, e
		}
		out.Targets = append(out.Targets, t)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}

// ProxySecretFor resolves a Key override and supplier default in one snapshot.
// A broken reference is an error, never an implicit direct connection.
func (s *Store) ProxySecretFor(provider, key string) (*ProxySecret, error) {
	tx, e := s.db.Begin()
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var exists int
	if e = tx.QueryRow(`SELECT count(*) FROM providers WHERE id=?`, provider).Scan(&exists); e != nil {
		return nil, e
	}
	if exists == 0 {
		return nil, ErrNotFound
	}
	if key != "" {
		if e = tx.QueryRow(`SELECT count(*) FROM provider_keys WHERE provider_id=? AND id=?`, provider, key).Scan(&exists); e != nil {
			return nil, e
		}
		if exists == 0 {
			return nil, ErrNotFound
		}
	}
	mode, id := "inherit", ""
	if key != "" {
		e = tx.QueryRow(`SELECT mode,COALESCE(proxy_id,'') FROM key_proxy WHERE provider_id=? AND key_id=?`, provider, key).Scan(&mode, &id)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return nil, e
		}
	}
	if mode == "inherit" {
		mode = "direct"
		e = tx.QueryRow(`SELECT mode,COALESCE(proxy_id,'') FROM provider_proxy WHERE provider_id=?`, provider).Scan(&mode, &id)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return nil, e
		}
	}
	if mode == "direct" {
		return nil, nil
	}
	if mode != "proxy" || id == "" {
		return nil, ErrInvalid
	}
	p := &ProxySecret{}
	e = tx.QueryRow(`SELECT id,name,kind,revision,secret FROM proxy_profiles WHERE id=?`, id).Scan(&p.Profile.ID, &p.Profile.Name, &p.Profile.Kind, &p.Profile.Revision, &p.Secret)
	if errors.Is(e, sql.ErrNoRows) {
		e = ErrNotFound
	}
	return p, e
}

// Secret normalization/validation belongs to the egress boundary; this layer enforces references and CAS.
func (s *Store) MutateProxy(in ProxyAction, normalize func(string, string, string) (string, error)) error {
	if !in.Approved || in.ExpectedRevision < 1 {
		return ErrInvalid
	}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var rev int64
	if e = tx.QueryRow(`SELECT revision FROM proxy_revision`).Scan(&rev); e != nil {
		return e
	}
	if rev != in.ExpectedRevision {
		return ErrConflict
	}
	switch in.Action {
	case "put":
		p := in.Profile
		if !ValidID(p.ID) || strings.TrimSpace(p.Name) == "" || len(p.Name) > 100 || strings.ContainsAny(p.Name, "\r\n") || (p.Kind != "http" && p.Kind != "socks5" && p.Kind != "mihomo") {
			return ErrInvalid
		}
		var old ProxySecret
		err := tx.QueryRow(`SELECT kind,secret FROM proxy_profiles WHERE id=?`, p.ID).Scan(&old.Profile.Kind, &old.Secret)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		secret := ""
		if in.Secret != nil {
			secret = *in.Secret
		} else if err == nil && old.Profile.Kind == p.Kind {
			secret = old.Secret
		} else {
			return ErrInvalid
		}
		secret, e = normalize(p.Kind, secret, in.NodeName)
		if e != nil {
			return ErrInvalid
		}
		var count int
		if e = tx.QueryRow(`SELECT count(*) FROM proxy_profiles`).Scan(&count); e != nil {
			return e
		}
		if errors.Is(err, sql.ErrNoRows) && count >= 64 {
			return ErrInvalid
		}
		_, e = tx.Exec(`INSERT INTO proxy_profiles(id,name,kind,revision,secret) VALUES(?,?,?,1,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,kind=excluded.kind,revision=proxy_profiles.revision+1,secret=excluded.secret`, p.ID, p.Name, p.Kind, secret)
	case "delete":
		var count int
		e = tx.QueryRow(`SELECT (SELECT count(*) FROM provider_proxy WHERE proxy_id=?)+(SELECT count(*) FROM key_proxy WHERE proxy_id=?)`, in.Profile.ID, in.Profile.ID).Scan(&count)
		if e != nil {
			return e
		}
		if count > 0 {
			return ErrReferenced
		}
		r, err := tx.Exec(`DELETE FROM proxy_profiles WHERE id=?`, in.Profile.ID)
		e = err
		if e == nil {
			n, _ := r.RowsAffected()
			if n == 0 {
				return ErrNotFound
			}
		}
	case "bind":
		b := in.Binding
		if b.Mode != "direct" && b.Mode != "proxy" && !(b.KeyID != "" && b.Mode == "inherit") {
			return ErrInvalid
		}
		if b.Mode == "proxy" {
			var n int
			if e = tx.QueryRow(`SELECT count(*) FROM proxy_profiles WHERE id=?`, b.ProxyID).Scan(&n); e != nil {
				return e
			}
			if n != 1 {
				return ErrNotFound
			}
		} else if b.ProxyID != "" {
			return ErrInvalid
		}
		var n int
		if b.KeyID == "" {
			e = tx.QueryRow(`SELECT count(*) FROM providers WHERE id=?`, b.ProviderID).Scan(&n)
		} else {
			e = tx.QueryRow(`SELECT count(*) FROM provider_keys WHERE provider_id=? AND id=?`, b.ProviderID, b.KeyID).Scan(&n)
		}
		if e != nil {
			return e
		}
		if n != 1 {
			return ErrNotFound
		}
		var ref any
		if b.Mode == "proxy" {
			ref = b.ProxyID
		}
		if b.KeyID == "" {
			_, e = tx.Exec(`INSERT INTO provider_proxy VALUES(?,?,?) ON CONFLICT(provider_id) DO UPDATE SET mode=excluded.mode,proxy_id=excluded.proxy_id`, b.ProviderID, b.Mode, ref)
		} else {
			_, e = tx.Exec(`INSERT INTO key_proxy VALUES(?,?,?,?) ON CONFLICT(provider_id,key_id) DO UPDATE SET mode=excluded.mode,proxy_id=excluded.proxy_id`, b.ProviderID, b.KeyID, b.Mode, ref)
		}
	default:
		return ErrInvalid
	}
	if e != nil {
		return e
	}
	if _, e = tx.Exec(`UPDATE proxy_revision SET revision=revision+1`); e != nil {
		return e
	}
	return tx.Commit()
}
