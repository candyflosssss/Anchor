package managed

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/candyflosssss/Anchor/internal/state"
)

func catalogName(id string) string { return "anchor_" + id + ".models.json" }

func buildCatalog(models []state.Model, version string) ([]byte, []string, error) {
	return buildCatalogTemplates(models, version, nil)
}

func buildCatalogTemplates(models []state.Model, version string, templates map[string]json.RawMessage) ([]byte, []string, error) {
	entries := []map[string]json.RawMessage{}
	ids := []string{}
	metadataBytes := 0
	for _, model := range models {
		// all-mapped is a model-name catalog, not a claim of official model
		// capabilities. Unknown instructions/context/efforts remain unspecified;
		// Codex keeps its built-in instruction defaults. Never clone an unrelated
		// model's instruction template just to make an alias visible.
		var entry map[string]json.RawMessage
		if version == "" && model.Codex == nil {
			raw, _ := json.Marshal(map[string]any{
				"display_name": model.ID, "description": "Anchor mapped model; upstream capabilities are not verified",
				"supported_reasoning_levels": []any{}, "shell_type": "default", "priority": 0,
				"support_verbosity": false, "experimental_supported_tools": []string{},
				"truncation_policy": map[string]any{"mode": "tokens", "limit": 10000},
				"input_modalities":  []string{"text"}, "context_window": nil,
			})
			_ = json.Unmarshal(raw, &entry)
			// Exact public ID first, otherwise one unambiguous upstream ID.
			// These entries come only from the operator's original local catalog.
			source := model.ID
			if _, ok := templates[source]; !ok {
				source = ""
				for _, candidate := range model.Candidates {
					if _, ok := templates[candidate.Model]; ok {
						if source != "" && source != candidate.Model {
							return nil, nil, errors.New("ambiguous local Codex templates")
						}
						source = candidate.Model
					}
				}
			}
			if template := templates[source]; template != nil {
				metadataBytes += len(template)
				if metadataBytes > maxConfig {
					return nil, nil, errors.New("generated catalog exceeds 1 MiB")
				}
				var supplied map[string]json.RawMessage
				if json.Unmarshal(template, &supplied) != nil || supplied == nil {
					return nil, nil, ErrUnsafe
				}
				for k, value := range supplied {
					entry[k] = value
				}
			}
		} else {
			if model.Codex == nil || (version != "" && model.Codex.Version != version) {
				return nil, nil, errors.New("version-matched Codex metadata required for every catalog model")
			}
			metadataBytes += len(model.Codex.Entry)
			if metadataBytes > maxConfig {
				return nil, nil, errors.New("generated catalog exceeds 1 MiB")
			}
			if err := model.Codex.Validate(); err != nil {
				return nil, nil, err
			}
			if err := json.Unmarshal(model.Codex.Entry, &entry); err != nil {
				return nil, nil, err
			}
		}
		set := func(k string, v any) { raw, _ := json.Marshal(v); entry[k] = raw }
		set("slug", model.ID)
		if model.DisplayName != "" {
			set("display_name", model.DisplayName)
		}
		// Apply explicit portable overrides to fields understood by the matched
		// Codex catalog. Instruction templates remain operator-supplied.
		if model.Settings != nil {
			cfg := model.Settings.Configuration
			if err := cfg.Validate(); err != nil {
				return nil, nil, err
			}
			if cfg.Limits.Context != nil {
				set("context_window", *cfg.Limits.Context)
			}
			if len(cfg.Capabilities.InputModalities) > 0 {
				modalities := []string{}
				for _, v := range cfg.Capabilities.InputModalities {
					if v == "text" || v == "image" {
						modalities = append(modalities, v)
					}
				}
				if len(modalities) > 0 {
					set("input_modalities", modalities)
				}
			} else if cfg.Capabilities.Vision != nil {
				modalities := []string{"text"}
				if *cfg.Capabilities.Vision {
					modalities = append(modalities, "image")
				}
				set("input_modalities", modalities)
			}
			if cfg.ParallelToolCalls != nil {
				set("supports_parallel_tool_calls", *cfg.ParallelToolCalls)
			}
			if cfg.Capabilities.Reasoning != nil && !*cfg.Capabilities.Reasoning {
				set("supported_reasoning_levels", []any{})
				set("default_reasoning_level", nil)
			}
			for _, option := range cfg.SupportedReasoning {
				if option.Type != "effort" {
					continue
				}
				levels := []map[string]string{}
				for _, value := range option.Values {
					if value == nil {
						return nil, nil, errors.New("Codex catalog cannot represent null reasoning effort")
					}
					switch *value {
					case "none", "minimal", "low", "medium", "high", "xhigh":
					default:
						return nil, nil, errors.New("reasoning effort is not supported by Codex export")
					}
					levels = append(levels, map[string]string{"effort": *value, "description": *value})
				}
				set("supported_reasoning_levels", levels)
				set("default_reasoning_level", nil)
				if cfg.DefaultReasoning != nil && cfg.DefaultReasoning.Type == "effort" {
					set("default_reasoning_level", cfg.DefaultReasoning.Value)
				}
			}
		}
		set("upgrade", nil)
		set("visibility", "list")
		set("supported_in_api", true)
		set("prefer_websockets", false)
		set("use_responses_lite", false)
		if err := validateCatalogEntry(entry); err != nil {
			return nil, nil, err
		}
		entries = append(entries, entry)
		ids = append(ids, model.ID)
	}
	if len(entries) == 0 {
		return nil, nil, errors.New("no compatible authorized catalog models")
	}
	raw, err := json.MarshalIndent(map[string]any{"models": entries}, "", "  ")
	if len(raw)+1 > maxConfig {
		return nil, nil, errors.New("generated catalog exceeds 1 MiB")
	}
	return append(raw, '\n'), ids, err
}

// fileReferences checks the configuration we restored and named profile files.
// If a reference cannot safely be inspected, recovery fails closed and keeps
// the artifact. It never reads a catalog's instructions as executable code.
func fileReferences(root, base string, predicate func(any) bool) (bool, error) {
	paths, err := filepath.Glob(filepath.Join(root, "*.config.toml"))
	if err != nil {
		return true, err
	}
	if len(paths) > 128 {
		return true, ErrUnsafe
	}
	if base != "" {
		paths = append(paths, base)
	}
	for _, path := range paths {
		raw, _, err := readConfig(root, path)
		if err != nil {
			return true, err
		}
		doc, err := document(raw)
		if err != nil {
			return true, err
		}
		if predicate(doc) {
			return true, nil
		}
	}
	return false, nil
}
func referencesCatalog(v any, path string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, v := range x {
			if k == "model_catalog_json" && sameCatalogPath(v, path) {
				return true
			}
			if referencesCatalog(v, path) {
				return true
			}
		}
	case []any:
		for _, v := range x {
			if referencesCatalog(v, path) {
				return true
			}
		}
	}
	return false
}

// Profile paths can spell the same artifact relatively or with dot segments.
func sameCatalogPath(value any, path string) bool {
	v, ok := value.(string)
	if !ok || v == "" {
		return false
	}
	if !filepath.IsAbs(v) {
		v = filepath.Join(filepath.Dir(path), v)
	}
	v, path = filepath.Clean(v), filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(v, path)
	}
	return v == path
}
