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
	entries := []map[string]json.RawMessage{}
	ids := []string{}
	metadataBytes := 0
	for _, model := range models {
		if model.Codex == nil || model.Codex.Version != version {
			return nil, nil, errors.New("version-matched Codex metadata required for every catalog model")
		}
		metadataBytes += len(model.Codex.Entry)
		if metadataBytes > maxConfig {
			return nil, nil, errors.New("generated catalog exceeds 1 MiB")
		}
		if err := model.Codex.Validate(); err != nil {
			return nil, nil, err
		}
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(model.Codex.Entry, &entry); err != nil {
			return nil, nil, err
		}
		set := func(k string, v any) { raw, _ := json.Marshal(v); entry[k] = raw }
		set("slug", model.ID)
		if model.DisplayName != "" {
			set("display_name", model.DisplayName)
		}
		// Only gateway-owned transport/selection properties are adjusted. Model
		// instructions, reasoning options, context sizes and tools are preserved.
		set("upgrade", nil)
		set("visibility", "list")
		set("supported_in_api", true)
		set("prefer_websockets", false)
		set("use_responses_lite", false)
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
