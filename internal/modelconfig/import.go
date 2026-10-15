package modelconfig

import "github.com/candyflosssss/Anchor/internal/modeldev"

// FillCatalog fills undeclared metadata without replacing explicit settings.
// Prices remain in the reference snapshot, never in upstream billing rates.
func FillCatalog(c ModelConfiguration, m modeldev.CatalogModel) ModelConfiguration {
	within := func(n, limit *int64) bool { return n == nil || limit == nil || *n <= *limit }
	if c.Limits.Context == nil && within(c.Limits.Input, m.Limits.Context) && within(c.Limits.Output, m.Limits.Context) && within(c.DefaultOutputTokens, m.Limits.Context) {
		c.Limits.Context = m.Limits.Context
	}
	if c.Limits.Input == nil && within(m.Limits.Input, c.Limits.Context) {
		c.Limits.Input = m.Limits.Input
	}
	if c.Limits.Output == nil && within(m.Limits.Output, c.Limits.Context) && within(c.DefaultOutputTokens, m.Limits.Output) {
		c.Limits.Output = m.Limits.Output
	}
	pairs := [][2]**bool{{&c.Capabilities.Vision, &m.Capabilities.Vision}, {&c.Capabilities.Attachment, &m.Capabilities.Attachment}, {&c.Capabilities.ToolCall, &m.Capabilities.ToolCall}, {&c.Capabilities.Reasoning, &m.Capabilities.Reasoning}, {&c.Capabilities.StructuredOutput, &m.Capabilities.StructuredOutput}, {&c.Capabilities.Temperature, &m.Capabilities.Temperature}}
	for _, p := range pairs {
		if *p[0] == nil {
			*p[0] = *p[1]
		}
	}
	if len(c.Capabilities.InputModalities) == 0 {
		c.Capabilities.InputModalities = append([]string{}, m.Capabilities.InputModalities...)
	}
	if len(c.Capabilities.OutputModalities) == 0 {
		c.Capabilities.OutputModalities = append([]string{}, m.Capabilities.OutputModalities...)
	}
	if c.Capabilities.Vision != nil && len(c.Capabilities.InputModalities) > 0 {
		list := []string{}
		for _, v := range c.Capabilities.InputModalities {
			if v != "image" {
				list = append(list, v)
			}
		}
		if *c.Capabilities.Vision {
			list = append(list, "image")
		}
		c.Capabilities.InputModalities = list
	}
	if len(c.SupportedReasoning) == 0 && (c.Capabilities.Reasoning == nil || *c.Capabilities.Reasoning) {
		c.SupportedReasoning = append([]modeldev.ReasoningOption{}, m.ReasoningOptions...)
	}
	return c
}
