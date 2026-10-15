package managed

import (
	"fmt"

	"github.com/candyflosssss/Anchor/internal/state"
)

// dshModel projects portable declarations into llm-pi-ai's model profile. Do
// not invent effort aliases or turn an output capability into a request default.
// The upstream schema is shared by the Web and Desktop profiles.
func dshModel(m state.Model) (map[string]any, any, error) {
	name := m.DisplayName
	if name == "" {
		name = m.ID
	}
	entry := map[string]any{"id": m.ID, "name": name}
	if m.Settings == nil {
		return entry, nil, nil
	}
	c := m.Settings.Configuration
	if err := c.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid DSH model settings for %q: %w", m.ID, err)
	}
	if c.Limits.Context != nil {
		entry["contextWindow"] = *c.Limits.Context
	}
	// DSH maxTokens is both a capability and the default sent on requests.
	// Limits.Output alone therefore cannot be exported losslessly.
	if c.DefaultOutputTokens != nil {
		entry["maxTokens"] = *c.DefaultOutputTokens
	}
	input := []string{}
	for _, modality := range c.Capabilities.InputModalities {
		if modality == "text" || modality == "image" {
			input = append(input, modality)
		}
	}
	if len(input) == 0 && c.Capabilities.Vision != nil {
		input = append(input, "text")
		if *c.Capabilities.Vision {
			input = append(input, "image")
		}
	}
	if len(input) > 0 {
		entry["input"] = input
	}
	if c.Capabilities.Reasoning != nil && !*c.Capabilities.Reasoning {
		entry["reasoningEfforts"] = false
		return entry, nil, nil
	}
	efforts := map[string]string{}
	thinking := false
	for _, option := range c.SupportedReasoning {
		if option.Type != "effort" {
			continue
		}
		for _, value := range option.Values {
			if value == nil {
				continue // Native JSON null is not DSH's off/omit-parameter.
			}
			switch *value {
			case "off", "minimal", "low", "medium", "high", "xhigh", "max":
				efforts[*value] = *value
				thinking = thinking || *value != "off"
			}
		}
	}
	if !thinking {
		if len(c.SupportedReasoning) > 0 {
			// DSH rejects an empty or off-only dict. Explicit options with no
			// representable thinking level must not inherit unrelated catalog levels.
			entry["reasoningEfforts"] = false
		}
		return entry, nil, nil
	}
	entry["reasoningEfforts"] = efforts
	if d := c.DefaultReasoning; d != nil && d.Type == "effort" && d.Value != nil {
		if _, ok := efforts[*d.Value]; ok {
			return entry, *d.Value, nil
		}
	}
	return entry, nil, nil
}
