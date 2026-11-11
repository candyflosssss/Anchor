package managed

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/candyflosssss/Anchor/internal/state"
)

func catalogName(id string) string { return "anchor_" + id + ".models.json" }

func buildCatalog(models []state.Model, version string) ([]byte, []string, error) {
	return buildCatalogTemplates(models, version, nil)
}

func buildCatalogTemplates(models []state.Model, version string, templates map[string]json.RawMessage) ([]byte, []string, error) {
	return buildCatalogWithSettings(models, version, templates, state.DefaultCodexSettings())
}

func buildGatewayCatalog(g GatewayBinding) ([]byte, []string, error) {
	return buildCatalogWithFallback(g.Models, g.CatalogVersion, g.CatalogTemplates, g.CatalogFallback, codexCompactSettings(g))
}

// The catalog is Codex's only model-list source, so its presence follows the
// binding scope. Consent only adds Anchor-authored capabilities and compaction.
func codexCatalogPlanned(b state.Binding, g GatewayBinding) bool {
	return len(g.Models) > 0 && (b.ModelScope == "all-mapped" || g.CatalogVersion != "")
}

func codexOverridesEnabled(g GatewayBinding) bool {
	return codexCompactSettings(g).OverrideModelCatalog
}

func codexCompactSettings(g GatewayBinding) state.CodexSettings {
	if g.CodexSettings != nil {
		return *g.CodexSettings
	}
	return state.DefaultCodexSettings()
}

func buildCatalogWithSettings(models []state.Model, version string, templates map[string]json.RawMessage, settings state.CodexSettings) ([]byte, []string, error) {
	return buildCatalogWithFallback(models, version, templates, nil, settings)
}

func buildCatalogWithFallback(models []state.Model, version string, templates map[string]json.RawMessage, fallback json.RawMessage, settings state.CodexSettings) ([]byte, []string, error) {
	raw, ids, err := encodeCatalogTemplates(models, version, templates, fallback, settings)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrCatalogInvalid, err)
	}
	return raw, ids, nil
}

func encodeCatalogTemplates(models []state.Model, version string, templates map[string]json.RawMessage, fallback json.RawMessage, settings state.CodexSettings) ([]byte, []string, error) {
	entries := []map[string]json.RawMessage{}
	ids := []string{}
	cloned := []int{}
	metadataBytes := 0
	for _, model := range models {
		// Models without their own template follow CLIProxyAPI: clone a cached
		// Codex entry for client behaviour, strip GPT-only capabilities and use a
		// compact instruction. Unknown context stays unspecified. Without a cache
		// a conservative name-only entry is exported instead.
		var entry map[string]json.RawMessage
		if version == "" && model.Codex == nil {
			template, err := mappedTemplate(model, templates)
			if err != nil {
				return nil, nil, err
			}
			if template == nil && fallback != nil {
				if json.Unmarshal(fallback, &entry) != nil || entry == nil {
					return nil, nil, ErrUnsafe
				}
				genericCodexEntry(entry)
				cloned = append(cloned, len(entries))
			} else {
				raw, _ := json.Marshal(map[string]any{
					"supported_reasoning_levels": []any{}, "shell_type": "default", "priority": 0,
					"support_verbosity": false, "experimental_supported_tools": []string{},
					"truncation_policy": map[string]any{"mode": "tokens", "limit": 10000},
					"input_modalities":  []string{"text"}, "context_window": nil,
				})
				_ = json.Unmarshal(raw, &entry)
			}
			set := func(k string, v any) { raw, _ := json.Marshal(v); entry[k] = raw }
			set("display_name", model.ID)
			set("description", "Anchor mapped model; upstream capabilities are not verified")
			if template != nil {
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
		// Codex catalog. Instruction templates remain operator-supplied. Without
		// consent, native templates stay verbatim apart from list identity.
		if settings.OverrideModelCatalog && model.Settings != nil {
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
				// Portable settings describe upstream-native options, not the
				// Codex ReasoningEffort enum. Project only representable values;
				// max/default/null must neither poison the whole catalog nor be
				// relabelled as another effort. Keep the stored settings intact.
				for _, value := range option.Values {
					if codexReasoningEffort(value) {
						levels = append(levels, map[string]string{"effort": *value, "description": *value})
					}
				}
				set("supported_reasoning_levels", levels)
				set("default_reasoning_level", nil)
				if cfg.DefaultReasoning != nil && cfg.DefaultReasoning.Type == "effort" && codexReasoningEffort(cfg.DefaultReasoning.Value) {
					set("default_reasoning_level", cfg.DefaultReasoning.Value)
				}
			}
		}
		if settings.OverrideModelCatalog && settings.AutoCompact {
			var context *int64
			if raw, ok := entry["context_window"]; ok {
				if json.Unmarshal(raw, &context) != nil {
					return nil, nil, ErrUnsafe
				}
			}
			if context != nil && *context > 0 {
				// Integer arithmetic avoids floating point rounding and overflow.
				percent := int64(settings.CompactPercent)
				if percent < 1 || percent > 99 {
					return nil, nil, ErrUnsafe
				}
				limit := (*context/100)*percent + (*context%100)*percent/100
				if limit < 1 {
					limit = 1
				}
				set("auto_compact_token_limit", limit)
			}
		}
		set("upgrade", nil)
		set("visibility", "list")
		set("supported_in_api", true)
		set("prefer_websockets", false)
		set("use_responses_lite", false)
		if version == "" && model.Codex == nil {
			if err := ensureMappedInstructions(entry); err != nil {
				return nil, nil, err
			}
		}
		if err := validateCatalogEntry(entry); err != nil {
			return nil, nil, err
		}
		entries = append(entries, entry)
		ids = append(ids, model.ID)
	}
	if len(entries) == 0 {
		return nil, nil, errors.New("no compatible authorized catalog models")
	}
	if err := rankClonedEntries(entries, cloned, fallback); err != nil {
		return nil, nil, err
	}
	raw, err := json.MarshalIndent(map[string]any{"models": entries}, "", "  ")
	if len(raw)+1 > maxConfig {
		return nil, nil, errors.New("generated catalog exceeds 1 MiB")
	}
	return append(raw, '\n'), ids, err
}

// Codex rejects a catalog entry without instructions, and full cloned prompts
// push the catalog past Codex's 1 MiB limit. Same literal as CLIProxyAPI.
const mappedBaseInstructions = "You are Codex, a coding agent. You and the user share one workspace."

// genericCodexEntry adapts a cloned Codex entry for a model it does not know.
// Required keys stay present: Codex rejects the whole catalog when they are missing.
func genericCodexEntry(entry map[string]json.RawMessage) {
	set := func(k string, v any) { raw, _ := json.Marshal(v); entry[k] = raw }
	set("base_instructions", mappedBaseInstructions)
	set("model_messages", map[string]any{
		"instructions_template": mappedBaseInstructions, "instructions_variables": nil, "approvals": nil,
		"collaboration_modes": nil, "auto_review": nil, "permissions": nil, "multi_agent": nil,
	})
	set("apply_patch_tool_type", nil)
	set("tool_mode", "direct")
	set("supports_search_tool", false)
	set("service_tiers", []any{})
	set("additional_speed_tiers", []any{})
	set("availability_nux", nil)
	for _, k := range []string{"supports_reasoning_effort_updates", "supports_image_detail_original"} {
		if _, ok := entry[k]; ok {
			set(k, false)
		}
	}
	set("context_window", nil)
	delete(entry, "max_context_window")
}

// Cloned entries list after every templated model, alphabetically, as in CLIProxyAPI.
func rankClonedEntries(entries []map[string]json.RawMessage, cloned []int, fallback json.RawMessage) error {
	if len(cloned) == 0 {
		return nil
	}
	var template map[string]json.RawMessage
	if json.Unmarshal(fallback, &template) != nil {
		return ErrUnsafe
	}
	priority := func(raw json.RawMessage) (int64, error) {
		var v int64
		return v, json.Unmarshal(raw, &v)
	}
	base, err := priority(template["priority"])
	if err != nil {
		return ErrUnsafe
	}
	isCloned := map[int]bool{}
	for _, i := range cloned {
		isCloned[i] = true
	}
	for i, entry := range entries {
		if isCloned[i] {
			continue
		}
		p, err := priority(entry["priority"])
		if err != nil {
			return ErrUnsafe
		}
		base = max(base, p)
	}
	name := func(i int) (string, string) {
		var display, slug string
		_ = json.Unmarshal(entries[i]["display_name"], &display)
		_ = json.Unmarshal(entries[i]["slug"], &slug)
		return strings.ToLower(display), slug
	}
	order := append([]int(nil), cloned...)
	sort.SliceStable(order, func(a, b int) bool {
		leftName, leftSlug := name(order[a])
		rightName, rightSlug := name(order[b])
		if leftName != rightName {
			return leftName < rightName
		}
		return leftSlug < rightSlug
	})
	for rank, i := range order {
		raw, _ := json.Marshal(base + int64(100*(rank+1)))
		entries[i]["priority"] = raw
	}
	return nil
}

// Exact public ID first, otherwise one unambiguous upstream ID. Never infer a
// source from a model family, similar name, speed suffix or default selection.
func mappedTemplate(model state.Model, templates map[string]json.RawMessage) (json.RawMessage, error) {
	if template, resolved := templates[model.ID]; resolved {
		return template, nil
	}
	source := ""
	for _, candidate := range model.Candidates {
		if templates[candidate.Model] != nil {
			if source != "" && source != candidate.Model {
				return nil, errors.New("ambiguous local Codex templates")
			}
			source = candidate.Model
		}
	}
	return templates[source], nil
}

func ensureMappedInstructions(entry map[string]json.RawMessage) error {
	var base *string
	if raw, ok := entry["base_instructions"]; ok && json.Unmarshal(raw, &base) != nil {
		return ErrUnsafe
	}
	var messages *struct {
		Template string `json:"instructions_template"`
	}
	if raw, ok := entry["model_messages"]; ok && json.Unmarshal(raw, &messages) != nil {
		return ErrUnsafe
	}
	if (base == nil || *base == "") && messages == nil {
		raw, _ := json.Marshal(mappedBaseInstructions)
		entry["base_instructions"] = raw
	}
	return nil
}

// Do not infer equivalence between an upstream-native effort and a Codex one.
func codexReasoningEffort(value *string) bool {
	if value == nil {
		return false
	}
	switch *value {
	case "none", "minimal", "low", "medium", "high", "xhigh":
		return true
	default:
		return false
	}
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
