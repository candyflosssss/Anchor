package core

import (
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/protocol"
)

func TestOutputTPSMeasurements(t *testing.T) {
	start := time.Now()
	complete := protocol.Observation{FirstOutput: start.Add(2 * time.Second), Terminal: start.Add(4 * time.Second), Status: "completed"}
	known := protocol.Usage{Known: true, Output: 25, ReasoningKnown: true, Reasoning: 5}
	for _, tc := range []struct {
		name                   string
		mutate                 func(*protocol.Observation, *protocol.Usage)
		protocol               string
		fail, interval, tokens bool
		want                   int64
	}{
		{name: "text", mutate: func(o *protocol.Observation, _ *protocol.Usage) { o.FirstText = o.FirstOutput }, interval: true, tokens: true, want: 20},
		{name: "tool_only", mutate: func(o *protocol.Observation, _ *protocol.Usage) { o.NonText = true }, interval: true, tokens: true, want: 20},
		{name: "mixed_tool_before_text", mutate: func(o *protocol.Observation, _ *protocol.Usage) {
			o.NonText = true
			o.FirstText = start.Add(3 * time.Second)
		}, interval: true, tokens: true, want: 20},
		{name: "missing_usage", mutate: func(_ *protocol.Observation, u *protocol.Usage) { u.Known = false }, interval: true},
		{name: "missing_reasoning", mutate: func(_ *protocol.Observation, u *protocol.Usage) { u.ReasoningKnown = false }, interval: true},
		{name: "anthropic_without_thinking", protocol: "anthropic", mutate: func(_ *protocol.Observation, u *protocol.Usage) { u.ReasoningKnown = false; u.Reasoning = 0 }, interval: true, tokens: true, want: 25},
		{name: "anthropic_thinking_unknown", protocol: "anthropic", mutate: func(o *protocol.Observation, u *protocol.Usage) { o.Reasoning = true; u.ReasoningKnown = false }, interval: true},
		{name: "reasoning_only", mutate: func(o *protocol.Observation, _ *protocol.Usage) { o.FirstOutput = time.Time{}; o.Reasoning = true }},
		{name: "no_terminal", mutate: func(o *protocol.Observation, _ *protocol.Usage) { o.Terminal = time.Time{} }},
		{name: "incomplete", mutate: func(o *protocol.Observation, _ *protocol.Usage) { o.Status = "incomplete" }},
		{name: "cancelled_or_write_failed", fail: true},
		{name: "zero_interval", mutate: func(o *protocol.Observation, _ *protocol.Usage) { o.Terminal = o.FirstOutput }},
		{name: "negative_interval", mutate: func(o *protocol.Observation, _ *protocol.Usage) { o.Terminal = start }},
		{name: "inconsistent_usage", mutate: func(_ *protocol.Observation, u *protocol.Usage) { u.Reasoning = 26 }, interval: true},
		{name: "negative_reasoning", mutate: func(_ *protocol.Observation, u *protocol.Usage) { u.Reasoning = -1 }, interval: true},
		{name: "explicit_zero_output", mutate: func(_ *protocol.Observation, u *protocol.Usage) { u.Output = 0; u.Reasoning = 0 }, interval: true, tokens: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obs, u := complete, known
			if tc.mutate != nil {
				tc.mutate(&obs, &u)
			}
			p := tc.protocol
			if p == "" {
				p = "responses"
			}
			tr := &requestTrace{start: start}
			setLogMetrics(tr, obs, u, p, !tc.fail)
			m := tr.row.Metrics
			if m.OutputIntervalComplete != tc.interval || (m.OutputGenerationMS != nil) != tc.interval || (m.NonReasoningOutputTokens != nil) != tc.tokens {
				t.Fatalf("unexpected output measurements: %+v", m)
			}
			if tc.interval && *m.OutputGenerationMS != 2000 {
				t.Fatalf("interval must exclude initial wait, not start at first text: %d", *m.OutputGenerationMS)
			}
			if tc.tokens && *m.NonReasoningOutputTokens != tc.want {
				t.Fatalf("unexpected non-reasoning output: %d", *m.NonReasoningOutputTokens)
			}
			if obs.NonText && m.OutputTextTokens != nil {
				t.Fatal("tool usage must not be attributed to body text")
			}
		})
	}
}
