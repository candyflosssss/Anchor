package modeldev

import (
	"context"
	"encoding/json"
)

// Resolve uses exact IDs only. A provider-qualified ID or an explicit canonical
// identity can disambiguate listings; otherwise multiple providers are unsafe.
func (c *Client) Resolve(ctx context.Context, id string) (ModelCatalog, error) {
	s, stale, err := c.load(ctx, false)
	if err != nil {
		return ModelCatalog{}, err
	}
	if stale {
		return ModelCatalog{}, ErrUnavailable
	}
	var matches, qualified, canonical []CatalogModel
	for pid, p := range s.data {
		for mid, m := range p.Models {
			if pid+"/"+mid == id {
				qualified = append(qualified, m)
			}
			if mid == id {
				matches = append(matches, m)
				if m.CanonicalModelID == pid+"/"+mid {
					canonical = append(canonical, m)
				}
			}
		}
	}
	selected := matches
	if len(qualified) > 0 {
		selected = qualified
	} else if len(canonical) > 0 {
		// A directory-declared canonical owner wins over unannotated reseller
		// listings. Explicitly conflicting identities still block inference;
		// never infer an owner from model-name prefixes.
		if len(canonical) != 1 {
			return ModelCatalog{}, ErrNotFound
		}
		for _, m := range matches {
			if m.CanonicalModelID != "" && m.CanonicalModelID != canonical[0].CanonicalModelID {
				return ModelCatalog{}, ErrNotFound
			}
		}
		selected = canonical
	}
	if len(selected) != 1 {
		return ModelCatalog{}, ErrNotFound
	}
	out := ModelCatalog{Source: "models.dev", FetchedAt: s.fetched.Unix(), ExpiresAt: s.fetched.Add(CacheTTL).Unix(), Version: s.version, Providers: []CatalogProvider{}, Models: selected, Total: 1}
	// Do not expose mutable cached pointers to configuration writers.
	raw, err := json.Marshal(out)
	if err != nil {
		return ModelCatalog{}, err
	}
	var copy ModelCatalog
	err = json.Unmarshal(raw, &copy)
	return copy, err
}
