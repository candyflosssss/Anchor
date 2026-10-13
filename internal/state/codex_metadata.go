package state

import (
	"encoding/json"
	"errors"
	"regexp"
)

// CodexMetadata is operator-supplied model metadata from a version-matched
// Codex catalog. Anchor preserves instructions/capabilities; it never guesses
// them from an alias or fetches a catalog/upstream during configuration writes.
type CodexMetadata struct {
	Version string          `json:"version"`
	Entry   json.RawMessage `json:"entry"`
}

var codexVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func ValidCodexVersion(v string) bool { return codexVersion.MatchString(v) }

func (c CodexMetadata) Validate() error {
	bad := errors.New("invalid Codex metadata: version and complete model catalog entry required")
	if !ValidCodexVersion(c.Version) || len(c.Entry) == 0 || len(c.Entry) > 256<<10 {
		return bad
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(c.Entry, &m) != nil || m == nil {
		return bad
	}
	for _, k := range []string{"slug", "display_name", "shell_type", "visibility"} {
		var v string
		if json.Unmarshal(m[k], &v) != nil || v == "" {
			return bad
		}
	}
	var shell, visibility string
	_ = json.Unmarshal(m["shell_type"], &shell)
	_ = json.Unmarshal(m["visibility"], &visibility)
	switch shell {
	case "default", "local", "shell_command", "unified_exec", "disabled":
	default:
		return bad
	}
	if visibility != "list" && visibility != "hide" && visibility != "none" {
		return bad
	}
	for _, k := range []string{"supported_in_api", "support_verbosity"} {
		var v bool
		if len(m[k]) == 0 || string(m[k]) == "null" || json.Unmarshal(m[k], &v) != nil {
			return bad
		}
	}
	// Current Codex ModelInfo no longer serializes this legacy field. Accept
	// omission without assigning a capability; validate older entries if present.
	if raw, ok := m["supports_parallel_tool_calls"]; ok {
		var v *bool
		if json.Unmarshal(raw, &v) != nil || v == nil {
			return bad
		}
	}
	var priority *int32
	if json.Unmarshal(m["priority"], &priority) != nil || priority == nil {
		return bad
	}
	var levels []struct {
		Effort      string  `json:"effort"`
		Description *string `json:"description"`
	}
	if json.Unmarshal(m["supported_reasoning_levels"], &levels) != nil || levels == nil {
		return bad
	}
	for _, level := range levels {
		if level.Effort == "" || level.Description == nil {
			return bad
		}
	}
	var tools []string
	if json.Unmarshal(m["experimental_supported_tools"], &tools) != nil || tools == nil {
		return bad
	}
	for _, tool := range tools {
		if tool == "" {
			return bad
		}
	}
	var trunc struct {
		Mode  string `json:"mode"`
		Limit int64  `json:"limit"`
	}
	if json.Unmarshal(m["truncation_policy"], &trunc) != nil || (trunc.Mode != "tokens" && trunc.Mode != "bytes") || trunc.Limit <= 0 {
		return bad
	}
	// Codex represents an unknown context window by omission or null. Preserve
	// it exactly; Anchor does not infer or fill a context size from the alias.
	if raw, ok := m["context_window"]; ok {
		var context *int64
		if json.Unmarshal(raw, &context) != nil || (context != nil && *context <= 0) {
			return bad
		}
	}
	// Complete instruction templates must be supplied by the operator, not
	// fabricated by Anchor. Either representation is accepted by Codex.
	var base string
	hasInstructions := json.Unmarshal(m["base_instructions"], &base) == nil && base != ""
	var messages struct {
		Template string `json:"instructions_template"`
	}
	if json.Unmarshal(m["model_messages"], &messages) == nil && messages.Template != "" {
		hasInstructions = true
	}
	if !hasInstructions {
		return bad
	}
	return nil
}

// ImportCodexMetadata copies exactly one entry; callers explicitly attest to
// version/upstream capability matching. Ambiguous slugs and oversized inputs fail.
func ImportCodexMetadata(raw []byte, slug, version string) (*CodexMetadata, error) {
	if len(raw) > 1<<20 || slug == "" {
		return nil, errors.New("catalog too large or source model missing")
	}
	var catalog struct {
		Models []json.RawMessage `json:"models"`
	}
	if json.Unmarshal(raw, &catalog) != nil || len(catalog.Models) == 0 {
		return nil, errors.New("catalog requires a models array")
	}
	var found *CodexMetadata
	for _, entry := range catalog.Models {
		var model struct {
			Slug string `json:"slug"`
		}
		if json.Unmarshal(entry, &model) != nil {
			return nil, errors.New("invalid catalog entry")
		}
		if model.Slug != slug {
			continue
		}
		if found != nil {
			return nil, errors.New("duplicate source model")
		}
		found = &CodexMetadata{Version: version, Entry: append(json.RawMessage(nil), entry...)}
	}
	if found == nil {
		return nil, errors.New("source model not found in catalog")
	}
	if err := found.Validate(); err != nil {
		return nil, err
	}
	return found, nil
}
