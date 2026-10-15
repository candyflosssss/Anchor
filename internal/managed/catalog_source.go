package managed

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/candyflosssss/Anchor/internal/state"
)

// Read only the catalog explicitly referenced by the pre-management config.
// Never use Anchor's last generated catalog as the next metadata baseline:
// doing that would retain removed portable overrides indefinitely.
func (m *Manager) codexTemplates(b state.Binding) (map[string]json.RawMessage, error) {
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

// Local catalogs may omit instructions (Codex uses its own defaults), unlike
// strict version-attested metadata. Check the fields we copy without inventing
// an instruction template or treating an arbitrary JSON object as ModelInfo.
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
