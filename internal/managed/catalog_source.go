package managed

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/candyflosssss/Anchor/internal/state"
)

// Prefer the explicitly configured catalog; otherwise reuse an exact match in
// Codex's validated local cache. Clean rebuilds skip the discarded configuration.
// The returned fallback is the cache entry cloned for models without a match.
func (m *Manager) codexTemplates(b state.Binding, models []state.Model, useConfigured bool) (map[string]json.RawMessage, json.RawMessage, error) {
	var templates map[string]json.RawMessage
	if useConfigured {
		var err error
		templates, err = m.configuredCodexTemplates(b)
		if err != nil {
			return nil, nil, err
		}
	}
	version, cached, err := localCatalog(b.ConfigRoot)
	if err != nil || (b.CodexVersion != "" && b.CodexVersion != version) {
		return templates, nil, nil
	}
	cache := make(map[string]json.RawMessage, len(cached))
	for slug, meta := range cached {
		cache[slug] = meta.Entry
	}
	resolved := make(map[string]json.RawMessage, len(models))
	for _, model := range models {
		if model.Codex != nil {
			continue
		}
		template, err := mappedTemplate(model, templates)
		if err != nil {
			return nil, nil, err
		}
		if template == nil {
			template, err = mappedTemplate(model, cache)
			if err != nil {
				return nil, nil, err
			}
		}
		// Keep negative resolutions too: a candidate ID might coincide with
		// another public alias whose template belongs to a different upstream.
		resolved[model.ID] = template
	}
	return resolved, fallbackTemplate(cache), nil
}

// Same choice as CLIProxyAPI: gpt-5.5 when cached, otherwise the entry Codex
// ranks first. Map order is random, so ties resolve by slug.
func fallbackTemplate(cache map[string]json.RawMessage) json.RawMessage {
	if template, ok := cache["gpt-5.5"]; ok {
		return template
	}
	var chosen json.RawMessage
	chosenSlug, chosenPriority := "", int64(0)
	for slug, template := range cache {
		var meta struct {
			Priority int64 `json:"priority"`
		}
		if json.Unmarshal(template, &meta) != nil {
			continue
		}
		if chosen == nil || meta.Priority < chosenPriority || (meta.Priority == chosenPriority && slug < chosenSlug) {
			chosen, chosenSlug, chosenPriority = template, slug, meta.Priority
		}
	}
	return chosen
}

// Read only the catalog explicitly referenced by the pre-management config.
// Never use Anchor's last generated catalog as the next metadata baseline:
// doing that would retain removed portable overrides indefinitely.
func (m *Manager) configuredCodexTemplates(b state.Binding) (map[string]json.RawMessage, error) {
	raw, _, err := readConfig(b.ConfigRoot, filepath.Join(b.ConfigRoot, "config.toml"))
	if err != nil {
		return nil, err
	}
	if j, e := m.Engine.load(b.ID); e == nil && j.Status != "restored" {
		raw = j.Original
	} else if e != nil && !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	doc, err := document(raw)
	if err != nil {
		return nil, err
	}
	path, _ := doc["model_catalog_json"].(string)
	if path == "" {
		return nil, nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(b.ConfigRoot, path)
	}
	path = filepath.Clean(path)
	// No network fetches, directory scans or symlink traversal. The existing
	// user file is never rewritten; only the Anchor-owned catalog is published.
	data, exists, err := readConfig(b.ConfigRoot, path)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	var catalog struct {
		Models []json.RawMessage `json:"models"`
	}
	if json.Unmarshal(data, &catalog) != nil || catalog.Models == nil || len(catalog.Models) > 128 {
		return nil, ErrUnsafe
	}
	out := map[string]json.RawMessage{}
	for _, entry := range catalog.Models {
		var identity struct {
			Slug string `json:"slug"`
		}
		if json.Unmarshal(entry, &identity) != nil || identity.Slug == "" || out[identity.Slug] != nil {
			return nil, ErrUnsafe
		}
		out[identity.Slug] = entry
	}
	return out, nil
}

// Generated entries must include instructions: Codex rejects the entire catalog
// otherwise. Only all-mapped name-only entries get the explicit generic fallback;
// version-attested metadata and operator templates retain their own instructions.
func validateCatalogEntry(entry map[string]json.RawMessage) error {
	for _, key := range []string{"slug", "display_name", "shell_type", "visibility"} {
		var value *string
		if json.Unmarshal(entry[key], &value) != nil || value == nil || *value == "" {
			return ErrUnsafe
		}
	}
	for _, key := range []string{"supported_in_api", "support_verbosity", "prefer_websockets", "use_responses_lite", "supports_parallel_tool_calls"} {
		if raw, ok := entry[key]; ok {
			var value *bool
			if json.Unmarshal(raw, &value) != nil || value == nil {
				return ErrUnsafe
			}
		}
	}
	var shape struct {
		Priority *int32 `json:"priority"`
		Levels   []struct {
			Effort      string  `json:"effort"`
			Description *string `json:"description"`
		} `json:"supported_reasoning_levels"`
		Tools        []string `json:"experimental_supported_tools"`
		Context      *int64   `json:"context_window"`
		Modalities   []string `json:"input_modalities"`
		Instructions *string  `json:"base_instructions"`
		Messages     *struct {
			Template string `json:"instructions_template"`
		} `json:"model_messages"`
		Truncation struct {
			Mode  string `json:"mode"`
			Limit int64  `json:"limit"`
		} `json:"truncation_policy"`
	}
	raw, err := json.Marshal(entry)
	if err != nil || json.Unmarshal(raw, &shape) != nil || shape.Priority == nil || shape.Levels == nil || shape.Tools == nil ||
		(shape.Messages != nil && shape.Messages.Template == "") ||
		((shape.Instructions == nil || *shape.Instructions == "") && shape.Messages == nil) ||
		(shape.Context != nil && *shape.Context <= 0) || shape.Truncation.Limit <= 0 || (shape.Truncation.Mode != "tokens" && shape.Truncation.Mode != "bytes") {
		return ErrUnsafe
	}
	for _, level := range shape.Levels {
		if level.Effort == "" || level.Description == nil {
			return ErrUnsafe
		}
	}
	return nil
}
