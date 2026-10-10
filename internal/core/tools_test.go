package core

import (
	"encoding/json"
	"testing"
)

func TestAgentExecutedTools(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		allowed bool
	}{
		{`{"type":"function","name":"shell"}`, true},
		{`{"type":"custom","name":"apply_patch"}`, true},
		{`{"type":"tool_search","execution":"client"}`, true},
		{`{"type":"namespace","name":"functions","tools":[{"type":"function","name":"shell"},{"type":"custom","name":"apply_patch"}]}`, true},
		{`{"type":"web_search"}`, false}, {`{"type":"image_generation"}`, false},
		{`{"type":"tool_search","execution":"server"}`, false}, {`{"type":"tool_search"}`, false},
		{`{"type":"namespace","name":"functions","tools":[{"type":"web_search"}]}`, false},
		{`{"type":"namespace","name":"functions","tools":[{"type":"tool_search","execution":"client"}]}`, false},
		{`{"type":"namespace","name":"functions","tools":[{"type":"namespace","name":"nested","tools":[{"type":"function"}]}]}`, false},
		{`{"type":"namespace","name":"functions","tools":[]}`, false},
		{`{"type":"namespace","tools":[{"type":"function"}]}`, false},
		{`null`, false}, {`[]`, false},
	} {
		if got := agentExecutedTool(json.RawMessage(tc.raw), false); got != tc.allowed {
			t.Fatal(tc.raw, got)
		}
	}
}
