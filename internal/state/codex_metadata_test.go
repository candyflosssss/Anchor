package state

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func fixtureMetadata(t *testing.T, version string) *CodexMetadata {
	t.Helper()
	raw, err := os.ReadFile("testdata/codex-models.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := ImportCodexMetadata(raw, "fixture-source", version)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func TestImportCodexMetadata(t *testing.T) {
	m := fixtureMetadata(t, "0.159.3")
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(m.Entry, &doc); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc["base_instructions"]), "Test fixture") {
		t.Fatal("instructions lost")
	}
	raw, _ := os.ReadFile("testdata/codex-models.json")
	for _, tc := range []struct {
		raw           []byte
		slug, version string
	}{
		{raw, "missing", "0.159.3"}, {raw, "fixture-source", "latest"},
		{[]byte(`{"models":[]}`), "fixture-source", "0.159.3"},
		{[]byte(`{"models":[` + string(m.Entry) + `,` + string(m.Entry) + `]}`), "fixture-source", "0.159.3"},
		{[]byte(strings.Repeat(" ", (1<<20)+1)), "fixture-source", "0.159.3"},
	} {
		if _, err := ImportCodexMetadata(tc.raw, tc.slug, tc.version); err == nil {
			t.Fatal("invalid catalog accepted")
		}
	}
	for _, key := range []string{"slug", "priority", "shell_type", "supported_in_api", "supported_reasoning_levels", "truncation_policy", "base_instructions"} {
		t.Run(key, func(t *testing.T) {
			clone := map[string]json.RawMessage{}
			for k, v := range doc {
				clone[k] = v
			}
			delete(clone, key)
			raw, _ := json.Marshal(clone)
			if (CodexMetadata{Version: m.Version, Entry: raw}).Validate() == nil {
				t.Fatal("incomplete metadata accepted")
			}
		})
	}
}

func TestRejectMalformedCatalogMetadata(t *testing.T) {
	m := fixtureMetadata(t, "0.159.3")
	for key, raw := range map[string]string{
		"priority": "null", "shell_type": `"unknown"`, "visibility": `"unknown"`,
		"supported_in_api": "null", "supported_reasoning_levels": `[{"effort":"medium"}]`,
		"experimental_supported_tools": `[null]`, "truncation_policy": `{"mode":"unknown","limit":1}`,
	} {
		var doc map[string]json.RawMessage
		json.Unmarshal(m.Entry, &doc)
		doc[key] = json.RawMessage(raw)
		encoded, _ := json.Marshal(doc)
		if (CodexMetadata{Version: m.Version, Entry: encoded}).Validate() == nil {
			t.Fatal(key, "accepted malformed metadata")
		}
	}
}

// Modern ModelInfo omits the legacy parallel-tool flag and permits an unknown
// context window. Preserve those omissions rather than inventing capabilities.
func TestCodexMetadataOptionalFields(t *testing.T) {
	original := fixtureMetadata(t, "0.159.3")
	for _, context := range []string{"", "null", "32000"} {
		t.Run("context="+context, func(t *testing.T) {
			var entry map[string]json.RawMessage
			json.Unmarshal(original.Entry, &entry)
			delete(entry, "supports_parallel_tool_calls")
			delete(entry, "context_window")
			if context != "" {
				entry["context_window"] = json.RawMessage(context)
			}
			raw, _ := json.Marshal(entry)
			catalog, _ := json.Marshal(map[string]any{"models": []json.RawMessage{raw}})
			imported, err := ImportCodexMetadata(catalog, "fixture-source", original.Version)
			if err != nil {
				t.Fatal(err)
			}
			if string(imported.Entry) != string(raw) {
				t.Fatal("metadata was rewritten")
			}
		})
	}
}

func TestCodexMetadataRejectsMalformedOptionalFields(t *testing.T) {
	original := fixtureMetadata(t, "0.159.3")
	for field, values := range map[string][]string{
		"supports_parallel_tool_calls": {"null", `"true"`, "1"},
		"context_window":               {"0", "-1", `"32000"`, "1.5"},
	} {
		for _, value := range values {
			var entry map[string]json.RawMessage
			json.Unmarshal(original.Entry, &entry)
			entry[field] = json.RawMessage(value)
			raw, _ := json.Marshal(entry)
			if (CodexMetadata{Version: original.Version, Entry: raw}).Validate() == nil {
				t.Fatalf("accepted %s=%s", field, value)
			}
		}
	}
}
