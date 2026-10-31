package protocol

import (
	"encoding/json"
	"fmt"
	"testing"
)

// These options travel together in Codex requests. Testing the whole set avoids
// fixing one rejection only to expose another in the next validator.
func TestAnthropicEgressEffortAndVerbosity(t *testing.T) {
	for _, client := range []string{"responses", "chat-completions"} {
		for _, effort := range []string{"none", "low", "medium", "high", "xhigh", "max"} {
			for _, verbosity := range []string{"low", "medium", "high"} {
				t.Run(fmt.Sprintf("%s/%s/%s", client, effort, verbosity), func(t *testing.T) {
					m := Object{"model": "public", "stream": true, "prompt_cache_key": "PRIVATE_CACHE"}
					if client == "responses" {
						m["input"] = "hello"
						m["reasoning"] = Object{"effort": effort, "summary": "auto"}
						m["text"] = Object{"format": Object{"type": "text"}, "verbosity": verbosity}
						m["client_metadata"] = Object{"private-key": "PRIVATE_METADATA"}
						m["include"] = []any{"reasoning.encrypted_content"}
						m["stream_options"] = Object{"reasoning_summary_delivery": "sequential_cutoff"}
					} else {
						m["messages"] = []any{Object{"role": "user", "content": "hello"}}
						m["reasoning_effort"], m["verbosity"] = effort, verbosity
						m["response_format"] = Object{"type": "text"}
					}
					raw, _ := json.Marshal(m)
					s := newTestSession(t, string(raw), client)
					a := prepareTestAttempt(t, s, "anthropic")
					wire, _ := Decode(a.RequestBody())
					if effort == "none" {
						if obj(wire["thinking"])["type"] != "disabled" || wire["output_config"] != nil {
							t.Fatal(wire)
						}
					} else if obj(wire["output_config"])["effort"] != effort || wire["thinking"] != nil {
						t.Fatal("effort was lost or thinking was invented", wire)
					}
					for _, key := range []string{"reasoning", "text", "verbosity", "prompt_cache_key", "client_metadata", "stream_options", "include"} {
						if _, exists := wire[key]; exists {
							t.Fatal("forwarded OpenAI option", key)
						}
					}
					if a.Diagnostics().Compatibility != Degraded || !hasDegradation(a.Diagnostics(), "text_verbosity_omitted") {
						t.Fatal(a.Diagnostics())
					}
					native := prepareTestAttempt(t, s, client)
					preserved, _ := Decode(native.RequestBody())
					if client == "responses" {
						if obj(preserved["reasoning"])["effort"] != effort || obj(preserved["text"])["verbosity"] != verbosity {
							t.Fatal("candidate polluted native options", preserved)
						}
					} else if preserved["reasoning_effort"] != effort || preserved["verbosity"] != verbosity {
						t.Fatal("candidate polluted native options", preserved)
					}
				})
			}
		}
	}
}
