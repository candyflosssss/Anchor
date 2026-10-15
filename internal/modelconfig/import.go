package modelconfig

import (
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"slices"
)

// FillCatalog fills undeclared metadata without replacing explicit settings.
// Prices remain in the reference snapshot, never in upstream billing rates.
func FillCatalog(c ModelConfiguration, m modeldev.CatalogModel) ModelConfiguration {
	within := func(n, limit *int64) bool { return n == nil || limit == nil || *n <= *limit }
	policy := modeldev.ContextPolicyFor(m)
	context := policy.RecommendedContext
	// A manually declared input/output budget can require a larger window.
	// Preserve those declarations; never silently reduce an explicit budget.
	if !within(c.Limits.Input, context) || !within(c.Limits.Output, context) || !within(c.DefaultOutputTokens, context) {
		context = m.Limits.Context
	}
	autoContext := c.Limits.Context == nil
	if autoContext && within(c.Limits.Input, context) && within(c.Limits.Output, context) && within(c.DefaultOutputTokens, context) {
		c.Limits.Context = context
	}
	input := m.Limits.Input
	// Only adapt an imported input ceiling to a newly selected price-aware
	// window. Existing manual limits retain the original conflict/skip behavior.
	if autoContext && input != nil && c.Limits.Context != nil && *input > *c.Limits.Context {
		input = c.Limits.Context
	}
	if c.Limits.Input == nil && within(input, c.Limits.Context) {
		c.Limits.Input = input
	}
	if c.Limits.Output == nil && within(m.Limits.Output, c.Limits.Context) && within(c.DefaultOutputTokens, m.Limits.Output) {
		c.Limits.Output = m.Limits.Output
	}
	// Explicit related declarations outrank directory booleans too. Otherwise
	// filling an unknown vision flag could silently remove a manual image input.
	if c.Capabilities.Vision == nil && len(c.Capabilities.InputModalities) > 0 {
		vision := slices.Contains(c.Capabilities.InputModalities, "image")
		c.Capabilities.Vision = &vision
	}
	if c.Capabilities.Reasoning == nil && len(c.SupportedReasoning) > 0 {
		yes := true
		c.Capabilities.Reasoning = &yes
	}
	if c.Capabilities.ToolCall == nil && c.ParallelToolCalls != nil && *c.ParallelToolCalls {
		yes := true
		c.Capabilities.ToolCall = &yes
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
