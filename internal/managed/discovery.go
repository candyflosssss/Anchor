package managed

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/candyflosssss/Anchor/internal/platform"
	"github.com/candyflosssss/Anchor/internal/state"
)

// Environment is supplied by the local host, never by a remote/UI path argument.
type Environment struct{ Home, CodexHome, Executable string }

func LocalEnvironment() (Environment, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Environment{}, err
	}
	exe, err := os.Executable()
	if err != nil {
		return Environment{}, err
	}
	return Environment{Home: home, CodexHome: os.Getenv("CODEX_HOME"), Executable: exe}, nil
}

type CatalogTemplate struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
}
type AgentTarget struct {
	ID             string            `json:"id"`
	Agent          string            `json:"agent"`
	ConfigRoot     string            `json:"config_root"`
	Source         string            `json:"source"`
	Supported      bool              `json:"supported"`
	Status         string            `json:"status"`
	CatalogVersion string            `json:"catalog_version"`
	Templates      []CatalogTemplate `json:"templates"`
}

func targetID(agent, root string) string {
	root = filepath.Clean(root)
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	sum := sha256.Sum256([]byte(root))
	return agent + "-" + hex.EncodeToString(sum[:6])
}

// Discover checks bounded, known configuration roots. Directory evidence does
// not claim a running/installed executable. No auth files, recursive disk scans,
// executable invocation, network requests or writes occur here.
func Discover(env Environment) ([]AgentTarget, error) {
	if !filepath.IsAbs(env.Home) {
		return nil, state.ErrInvalid
	}
	candidates := []struct{ agent, root, source string }{}
	if env.CodexHome != "" && filepath.IsAbs(env.CodexHome) {
		candidates = append(candidates, struct{ agent, root, source string }{"codex", env.CodexHome, "CODEX_HOME"})
	}
	candidates = append(candidates,
		struct{ agent, root, source string }{"codex", filepath.Join(env.Home, ".codex"), "home"},
		struct{ agent, root, source string }{"claude", filepath.Join(env.Home, ".claude"), "home"},
		struct{ agent, root, source string }{"gemini", filepath.Join(env.Home, ".gemini"), "home"},
		struct{ agent, root, source string }{"opencode", filepath.Join(env.Home, ".config", "opencode"), "home"},
	)
	if runtime.GOOS == "windows" {
		candidates = append(candidates, struct{ agent, root, source string }{"opencode", filepath.Join(env.Home, "AppData", "Roaming", "opencode"), "home"})
	}
	out := []AgentTarget{}
	seen := map[string]bool{}
	for _, c := range candidates {
		root := filepath.Clean(c.root)
		id := targetID(c.agent, root)
		if seen[id] {
			continue
		}
		seen[id] = true
		st, err := os.Lstat(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		t := AgentTarget{ID: id, Agent: c.agent, ConfigRoot: root, Source: c.source, Templates: []CatalogTemplate{}, Status: "unsupported"}
		if err != nil || !st.IsDir() || platform.CheckManagedPath(root, filepath.Join(root, "config.toml")) != nil {
			t.Status = "unsafe"
			out = append(out, t)
			continue
		}
		if c.agent != "codex" {
			out = append(out, t)
			continue
		}
		t.Supported = true
		if _, err := (Codex{}).Inspect(state.Binding{ConfigRoot: root}); err != nil {
			t.Status = "config_unsupported"
			out = append(out, t)
			continue
		}
		version, catalog, err := localCatalog(root)
		if err != nil {
			t.Status = "catalog_unavailable"
		} else {
			t.Status = "ready"
			t.CatalogVersion = version
			// Keep cache order stable for the UI; catalog map serialization is irrelevant.
			for _, meta := range catalog {
				var entry struct {
					Slug        string `json:"slug"`
					DisplayName string `json:"display_name"`
				}
				_ = json.Unmarshal(meta.Entry, &entry)
				t.Templates = append(t.Templates, CatalogTemplate{Slug: entry.Slug, DisplayName: entry.DisplayName})
			}
			sort.Slice(t.Templates, func(i, j int) bool { return t.Templates[i].Slug < t.Templates[j].Slug })
		}
		out = append(out, t)
	}
	return out, nil
}

func localCatalog(root string) (string, map[string]state.CodexMetadata, error) {
	path := filepath.Join(root, "models_cache.json")
	if err := platform.CheckManagedPath(root, path); err != nil {
		return "", nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		return "", nil, ErrUnsafe
	}
	var cache struct {
		ClientVersion string            `json:"client_version"`
		Models        []json.RawMessage `json:"models"`
	}
	if json.Unmarshal(raw, &cache) != nil || !state.ValidCodexVersion(cache.ClientVersion) || len(cache.Models) == 0 || len(cache.Models) > 128 {
		return "", nil, state.ErrInvalid
	}
	catalog := map[string]state.CodexMetadata{}
	for _, entry := range cache.Models {
		meta := state.CodexMetadata{Version: cache.ClientVersion, Entry: entry}
		var v struct {
			Slug string `json:"slug"`
		}
		if json.Unmarshal(entry, &v) != nil || meta.Validate() != nil {
			return "", nil, state.ErrInvalid
		}
		if _, duplicate := catalog[v.Slug]; duplicate {
			return "", nil, state.ErrInvalid
		}
		catalog[v.Slug] = meta
	}
	return cache.ClientVersion, catalog, nil
}
