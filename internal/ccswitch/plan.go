package ccswitch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/candyflosssss/Anchor/internal/state"
)

func short(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:6]) }
func identifier(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._-", r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
		if b.Len() > 60 {
			break
		}
	}
	return strings.Trim(b.String(), "._-")
}
func literal(s string) bool {
	if s == "" || strings.ContainsAny(s, "\r\n") || strings.HasPrefix(s, "!") || strings.Contains(s, "${") || strings.HasPrefix(s, "$") || strings.HasPrefix(s, "{env:") || strings.HasPrefix(s, "{file:") || strings.Contains(strings.ToLower(s), "proxy_managed") {
		return false
	}
	return true
}

type group struct {
	public  Provider
	private state.ProviderImport
	keys    map[string]string
	models  map[string][]string
}

func Build(ctx context.Context, path string, store *state.Store) (*Plan, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".cc-switch", "cc-switch.db")
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, e := os.UserHomeDir()
		if e != nil {
			return nil, e
		}
		path = filepath.Join(home, path[2:])
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, state.ErrInvalid
	}
	p := &Plan{Report: Report{DatabasePath: path, Providers: []Provider{}, Models: []Model{}, Issues: []Issue{}}}
	records, err := read(ctx, path)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			p.Report.Failure = "source_missing"
		case errors.Is(err, errLimit):
			p.Report.Failure = "source_limit"
		default:
			p.Report.Failure = "source_invalid"
		}
		return p, nil
	}
	existing, err := store.Providers()
	if err != nil {
		return nil, err
	}
	models, err := store.Models()
	if err != nil {
		return nil, err
	}
	groups := map[string]*group{}
	issue := func(r record, reason string) { p.Report.Issues = append(p.Report.Issues, Issue{r.App, r.Name, reason}) }
	for _, source := range records {
		if source.Config == nil {
			issue(source, "invalid_config")
			continue
		}
		expanded := []record{source}
		if source.App == "universal" {
			expanded = expandUniversal(source)
		}
		for _, r := range expanded {
			entries, reason := parseRecord(r)
			if reason != "" {
				issue(r, reason)
				continue
			}
			for _, e := range entries {
				if !state.ValidProtocol(e.Protocol) {
					issue(r, "unsupported_protocol")
					continue
				}
				if !literal(e.Secret) || len(e.Secret) > 8192 {
					issue(r, "missing_or_referenced_key")
					continue
				}
				urls := []string{strings.TrimRight(e.URL, "/")}
				for _, u := range r.Endpoints {
					u = strings.TrimRight(u, "/")
					found := false
					for _, old := range urls {
						if old == u {
							found = true
						}
					}
					if !found {
						urls = append(urls, u)
					}
				}
				valid := true
				for _, u := range urls {
					if state.ValidateURL(u) != nil {
						valid = false
					}
				}
				if !valid || len(urls) > state.MaxProviderURLs {
					issue(r, "invalid_endpoint")
					continue
				}
				// URL order is meaningful for Anchor's first request address.
				identity := strings.Join(urls, "\n")
				g := groups[identity]
				if g == nil {
					address, _ := url.Parse(urls[0])
					id := "ccs-" + identifier(address.Hostname()) + "-" + short(identity)
					homepage := r.Website
					if state.ValidateHomepage(homepage) != nil {
						homepage = ""
					}
					g = &group{public: Provider{ID: id, BaseURLs: urls, Sources: []Source{}, Keys: []Key{}, Status: "ready"}, private: state.ProviderImport{Provider: state.Provider{ID: id, BaseURLs: urls, Protocol: e.Protocol, HomepageURL: homepage, Enabled: true, TimeoutSeconds: 120, MaxConcurrent: 4, CostMultiplier: 1}}, keys: map[string]string{}, models: map[string][]string{}}
					groups[identity] = g
				}
				keyIdentity := e.Protocol + "\x00" + e.Secret
				keyID := g.keys[keyIdentity]
				if keyID == "" {
					if len(g.private.Keys) >= 64 {
						issue(r, "key_limit")
						continue
					}
					keyID = "ccs-" + r.App + "-" + short(r.ID+"\x00"+e.Protocol)
					// Separate model endpoints can share source identity. A group can
					// also contain both a universal record and its generated row.
					for _, k := range g.private.Keys {
						if k.ID == keyID {
							keyID += "-" + strconv.Itoa(len(g.private.Keys)+1)
							break
						}
					}
					g.keys[keyIdentity] = keyID
					g.private.Keys = append(g.private.Keys, state.InitialProviderKey{ID: keyID, Protocol: e.Protocol, Secret: e.Secret})
					g.public.Keys = append(g.public.Keys, Key{keyID, e.Protocol})
				}
				src := Source{r.App, r.Name}
				present := false
				for _, s := range g.public.Sources {
					if s == src {
						present = true
					}
				}
				if !present {
					g.public.Sources = append(g.public.Sources, src)
				}
				if len(e.Models) == 0 {
					issue(r, "models_not_configured")
				}
				for _, upstream := range e.Models {
					if upstream == "" || len(upstream) > 256 || strings.ContainsAny(upstream, "\r\n") {
						issue(r, "invalid_model")
						continue
					}
					ids := g.models[upstream]
					found := false
					for _, id := range ids {
						if id == keyID {
							found = true
						}
					}
					if !found {
						g.models[upstream] = append(ids, keyID)
					}
				}
			}
		}
	}
	existingIDs := map[string]bool{}
	for _, v := range existing {
		existingIDs[v.ID] = true
	}
	currentModels := map[string]state.Model{}
	for _, v := range models {
		currentModels[v.ID] = v
	}
	plannedModels := map[string]state.Model{}
	identities := make([]string, 0, len(groups))
	for id := range groups {
		identities = append(identities, id)
	}
	sort.Strings(identities)
	for _, identity := range identities {
		g := groups[identity]
		if existingIDs[g.public.ID] {
			g.public.Status = "exists"
			p.Report.Providers = append(p.Report.Providers, g.public)
			continue
		}
		p.Report.Providers = append(p.Report.Providers, g.public)
		p.Providers = append(p.Providers, g.private)
		upstreams := make([]string, 0, len(g.models))
		for model := range g.models {
			upstreams = append(upstreams, model)
		}
		sort.Strings(upstreams)
		for _, upstream := range upstreams {
			id := upstream
			if !state.ValidModelID(id) {
				id = "ccs-" + identifier(upstream) + "-" + short(upstream)
			}
			model, ok := plannedModels[id]
			if !ok {
				model, ok = currentModels[id]
				if !ok {
					model = state.Model{ID: id, DisplayName: upstream, Strategy: "round-robin", Tools: true, Streaming: true}
				}
			}
			if len(model.Candidates) >= 16 {
				p.Report.Issues = append(p.Report.Issues, Issue{"anchor", id, "candidate_limit"})
				continue
			}
			keys := g.models[upstream]
			model.Candidates = append(append([]state.Candidate(nil), model.Candidates...), state.Candidate{ProviderID: g.public.ID, Model: upstream, KeyIDs: keys})
			if model.Validate() != nil {
				p.Report.Issues = append(p.Report.Issues, Issue{"anchor", id, "invalid_model"})
				continue
			}
			plannedModels[id] = model
			p.Report.Models = append(p.Report.Models, Model{ID: id, ProviderID: g.public.ID, Upstream: upstream, KeyIDs: keys})
		}
	}
	ids := make([]string, 0, len(plannedModels))
	for id := range plannedModels {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		model := plannedModels[id]
		p.Models = append(p.Models, state.ImportedModel{Model: model, ExpectedRevision: currentModels[id].Revision})
	}
	// Includes source secrets and current target revisions without exposing them.
	// Rebuilding on apply pins the review to both database snapshots.
	raw, _ := json.Marshal(struct {
		Records   []record
		Providers []state.Provider
		Models    []state.Model
		Plan      *Plan
	}{records, existing, models, p})
	h := sha256.Sum256(raw)
	p.Report.Fingerprint = hex.EncodeToString(h[:])
	return p, nil
}
