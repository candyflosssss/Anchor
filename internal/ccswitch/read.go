package ccswitch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type record struct {
	ID, App, Name, Website string
	Config, Meta           map[string]any
	Endpoints              []string
}

var errLimit = errors.New("source_limit")

// One snapshot also sees committed WAL data. mode=ro forbids database writes;
// query_only is additional protection. No migrations or credential resolution.
func read(ctx context.Context, path string) ([]record, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("source_invalid")
	}
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro"}).String())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(ctx, `PRAGMA query_only=ON; PRAGMA busy_timeout=1000`); err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id,app_type,name,substr(settings_config,1,1048577),COALESCE(website_url,''),substr(COALESCE(meta,'{}'),1,1048577),length(settings_config)+length(COALESCE(meta,'')) FROM providers ORDER BY app_type,id LIMIT 4097`)
	if err != nil {
		return nil, err
	}
	items := []record{}
	total := 0
	for rows.Next() {
		var r record
		var config, meta string
		var size int
		if err = rows.Scan(&r.ID, &r.App, &r.Name, &config, &r.Website, &meta, &size); err != nil {
			rows.Close()
			return nil, err
		}
		total += size
		if len(items) >= 4096 || total > 16<<20 || size > 1<<20 {
			rows.Close()
			return nil, errLimit
		}
		// Malformed per-provider JSON is an inventory issue, not a reason to
		// discard other platforms. parse marks nil Config explicitly.
		_ = json.Unmarshal([]byte(config), &r.Config)
		if json.Unmarshal([]byte(meta), &r.Meta) != nil {
			r.Config = nil
		}
		items = append(items, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	hasTable := func(name string) bool {
		var n int
		_ = tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
		return n == 1
	}
	if hasTable("provider_endpoints") {
		rows, err = tx.QueryContext(ctx, `SELECT provider_id,app_type,url FROM provider_endpoints ORDER BY app_type,provider_id,id LIMIT 65537`)
		if err != nil {
			return nil, err
		}
		lookup := map[string]int{}
		for i, r := range items {
			lookup[r.App+"\x00"+r.ID] = i
		}
		n := 0
		for rows.Next() {
			var id, app, u string
			if err = rows.Scan(&id, &app, &u); err != nil {
				rows.Close()
				return nil, err
			}
			n++
			total += len(u)
			if n > 65536 || total > 16<<20 {
				rows.Close()
				return nil, errLimit
			}
			if i, ok := lookup[app+"\x00"+id]; ok {
				items[i].Endpoints = append(items[i].Endpoints, u)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	if hasTable("settings") {
		var raw string
		err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='universal_providers'`).Scan(&raw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if len(raw)+total > 16<<20 {
			return nil, errLimit
		}
		if raw != "" {
			var shared map[string]map[string]any
			if json.Unmarshal([]byte(raw), &shared) != nil {
				return nil, errors.New("source_invalid")
			}
			ids := make([]string, 0, len(shared))
			for id := range shared {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				v := shared[id]
				items = append(items, record{ID: id, App: "universal", Name: str(v, "name"), Website: str(v, "websiteUrl"), Config: v, Meta: obj(v, "meta")})
			}
		}
	}
	if len(items) > 4096 {
		return nil, errLimit
	}
	return items, nil
}
