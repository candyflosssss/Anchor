package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAnthropicOutputBudgetPrecedenceAndIsolation(t *testing.T) {
	for _, client := range []string{"responses", "chat-completions", "anthropic"} {
		for _, tc := range []struct {
			name        string
			policy      OutputTokenPolicy
			explicit    any
			hasExplicit bool
			want        int64
			source      string
		}{
			{"declared maximum", OutputTokenPolicy{Maximum: 128000}, nil, false, 128000, "model_limit"},
			{"configured default", OutputTokenPolicy{Default: 16000, Maximum: 128000}, nil, false, 16000, "model_default"},
			{"client smaller", OutputTokenPolicy{Default: 16000, Maximum: 128000}, float64(512), true, 512, "client"},
			{"client above declaration", OutputTokenPolicy{Maximum: 128000}, float64(256000), true, 256000, "client"},
			{"null uses declaration", OutputTokenPolicy{Maximum: 128000}, nil, true, 128000, "model_limit"},
			{"legacy without declarations", OutputTokenPolicy{}, nil, false, 4096, "legacy_fallback"},
		} {
			t.Run(client+"/"+tc.name, func(t *testing.T) {
				m := Object{"model": "public"}
				field := "max_output_tokens"
				if client == "responses" {
					m["input"] = "hello"
				} else {
					m["messages"] = []any{Object{"role": "user", "content": "hello"}}
					field = "max_tokens"
				}
				if tc.hasExplicit {
					m[field] = tc.explicit
				}
				raw, _ := json.Marshal(m)
				s := newTestSession(t, string(raw), client)
				a, err := s.Prepare(AttemptOptions{Protocol: "anthropic", Model: "up", OutputTokens: tc.policy})
				if err != nil {
					t.Fatal(err)
				}
				wire, _ := Decode(a.RequestBody())
				if wire["max_tokens"] != float64(tc.want) || a.Diagnostics().OutputTokens != tc.want || a.Diagnostics().OutputTokenSource != tc.source {
					t.Fatal(wire, a.Diagnostics())
				}
				// The policy is per attempt and cannot leak into other protocol candidates.
				if client != "anthropic" {
					native := prepareTestAttempt(t, s, client)
					preserved, _ := Decode(native.RequestBody())
					if tc.hasExplicit {
						if preserved[field] != tc.explicit {
							t.Fatal("client budget changed")
						}
					} else if _, exists := preserved[field]; exists {
						t.Fatal("injected into native OpenAI request")
					}
					other := "responses"
					if client == "responses" {
						other = "chat-completions"
					}
					b, err := s.Prepare(AttemptOptions{Protocol: other, OutputTokens: tc.policy})
					if err != nil {
						t.Fatal(err)
					}
					if b.Diagnostics().OutputTokens != 0 || b.Diagnostics().OutputTokenSource != "" {
						t.Fatal("Anthropic policy leaked")
					}
				}
			})
		}
	}
}

func TestAnthropicOutputLimitRemainsIncomplete(t *testing.T) {
	s := newTestSession(t, `{"input":"hello","max_output_tokens":12}`, "responses")
	a, err := s.Prepare(AttemptOptions{Protocol: "anthropic", OutputTokens: OutputTokenPolicy{Maximum: 128000}})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"id":"r","stop_reason":"max_tokens","content":[{"type":"text","text":"PARTIAL"}],"usage":{"input_tokens":1,"output_tokens":12}}`
	r, err := a.TranslateResponse(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.Body), `"status":"incomplete"`) || !strings.Contains(string(r.Body), `"reason":"max_output_tokens"`) || r.Diagnostics.OutputTokens != 12 {
		t.Fatal(fmt.Sprint(r.Diagnostics), string(r.Body))
	}
}
