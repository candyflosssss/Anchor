package protocol

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestFirstOutputNotificationExcludesLifecycleAndIncludesReasoningAndTools(t *testing.T) {
	for _, tc := range []struct{ protocol, prefix, output, terminal string }{
		{"responses", `{"type":"response.created","response":{"id":"r"}}`, `{"type":"response.reasoning_summary_text.delta","delta":"thought"}`, `{"type":"response.completed","response":{"status":"completed","output":[]}}`},
		{"chat-completions", `{"choices":[{"index":0,"delta":{"role":"assistant"}}]}`, `{"choices":[{"index":0,"delta":{"reasoning_content":"thought"}}]}`, `[DONE]`},
		{"anthropic", `{"type":"ping"}`, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`, `{"type":"message_stop"}`},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			request := `{"model":"public","messages":[{"role":"user","content":"hi"}],"stream":true}`
			if tc.protocol == "responses" {
				request = `{"model":"public","input":"hi","stream":true}`
			}
			if tc.protocol == "anthropic" {
				request = `{"model":"public","messages":[{"role":"user","content":"hi"}],"stream":true,"max_tokens":10}`
			}
			s, err := NewSession([]byte(request), tc.protocol)
			if err != nil {
				t.Fatal(err)
			}
			a, err := s.Prepare(AttemptOptions{Protocol: tc.protocol, Model: "actual", PublicModel: "public"})
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			raw := "data: " + tc.prefix + "\n\n: heartbeat\n\ndata: " + tc.output + "\n\ndata: " + tc.output + "\n\ndata: " + tc.terminal + "\n\n"
			_, err = a.TranslateStream(context.Background(), io.Discard, strings.NewReader(raw), StreamOptions{OnFirstOutput: func() { count++ }})
			if err != nil || count != 1 {
				t.Fatalf("count=%d err=%v", count, err)
			}
			count = 0
			raw = "data: " + tc.prefix + "\n\n: heartbeat\n\ndata: " + tc.terminal + "\n\n"
			_, err = a.TranslateStream(context.Background(), io.Discard, strings.NewReader(raw), StreamOptions{OnFirstOutput: func() { count++ }})
			if err != nil || count != 0 {
				t.Fatalf("envelope treated as output count=%d err=%v", count, err)
			}
		})
	}
}
