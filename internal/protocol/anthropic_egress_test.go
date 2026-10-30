package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAnthropicEgressOpenAIOptions(t *testing.T) {
	for _, client := range []string{"responses", "chat-completions"} {
		t.Run(client, func(t *testing.T) {
			m := Object{"model": "public", "stream": true}
			if client == "responses" {
				m["input"] = []any{Object{"role": "user", "content": "hello"}, Object{"type": "reasoning", "summary": []any{}, "encrypted_content": nil}, Object{"role": "assistant", "content": "previous answer"}, Object{"role": "user", "content": "continue"}}
				m["client_metadata"] = Object{"private-key": "PRIVATE_METADATA"}
				m["stream_options"] = Object{"reasoning_summary_delivery": "sequential_cutoff"}
				m["include"] = []any{"reasoning.encrypted_content"}
				m["truncation"] = "disabled"
				m["text"] = Object{"format": Object{"type": "text"}}
				m["reasoning"] = Object{"effort": "none", "summary": "auto"}
			} else {
				m["messages"] = []any{Object{"role": "user", "content": "hello"}}
				m["reasoning_effort"] = "none"
				m["response_format"] = Object{"type": "text"}
			}
			raw, _ := json.Marshal(m)
			s := newTestSession(t, string(raw), client)
			a := prepareTestAttempt(t, s, "anthropic")
			wire, _ := Decode(a.RequestBody())
			if obj(wire["thinking"])["type"] != "disabled" || wire["stream"] != true || wire["max_tokens"] != float64(4096) {
				t.Fatal(wire)
			}
			for _, key := range []string{"client_metadata", "stream_options", "include", "truncation", "text", "reasoning"} {
				if _, exists := wire[key]; exists {
					t.Fatalf("forwarded %s", key)
				}
			}
			if strings.Contains(string(a.RequestBody()), "PRIVATE_METADATA") {
				t.Fatal("forwarded client metadata")
			}
			d := a.Diagnostics()
			if client == "responses" {
				if len(arr(wire["messages"])) != 3 || len(d.Degradations) != 4 || d.Compatibility != Degraded {
					t.Fatal(wire, d)
				}
				for _, label := range []Degradation{"client_metadata_omitted", "reasoning_summary_delivery", SummaryStyle, EncryptedProjection} {
					if !hasDegradation(d, label) {
						t.Fatal(d)
					}
				}
			} else if d.Compatibility != Full {
				t.Fatal(d)
			}
			// Preparing another candidate must still see every original option.
			native := prepareTestAttempt(t, s, client)
			nativeWire, _ := Decode(native.RequestBody())
			if client == "responses" && (obj(nativeWire["client_metadata"])["private-key"] != "PRIVATE_METADATA" || len(arr(nativeWire["input"])) != 4) {
				t.Fatal("candidate polluted native history")
			}
			if client == "chat-completions" && nativeWire["reasoning_effort"] != "none" {
				t.Fatal("candidate polluted native options")
			}
		})
	}
}

func TestAnthropicEgressSafeOptionFailures(t *testing.T) {
	for _, tc := range []struct{ fields, reason string }{
		{`"private-field":"PRIVATE_VALUE"`, "request_fields"},
		{`"client_metadata":[]`, "request_fields"},
		{`"client_metadata":{"private-field":12}`, "request_fields"},
		{`"client_metadata":{"private-field":null}`, "request_fields"},
		{`"stream_options":[]`, "request_fields"},
		{`"stream_options":{"include_usage":true}`, "request_fields"},
		{`"stream_options":{"reasoning_summary_delivery":"PRIVATE_VALUE"}`, "request_fields"},
		{`"include":{}`, "include_options"},
		{`"include":[{}]`, "include_options"},
		{`"include":["PRIVATE_VALUE"]`, "include_options"},
		{`"truncation":"auto"`, "truncation"},
		{`"text":[]`, "text_format"},
		{`"text":{"format":{"type":"json_object"}}`, "text_format"},
		{`"text":{"format":{"type":"text","private-field":"PRIVATE_VALUE"}}`, "text_format"},
		{`"text":{"verbosity":"low"}`, "text_format"},
		{`"reasoning":[]`, "reasoning_options"},
		{`"reasoning":{"effort":"high"}`, "reasoning_options"},
		{`"reasoning":{"effort":{}}`, "reasoning_options"},
		{`"reasoning":{"summary":"PRIVATE_VALUE"}`, "reasoning_options"},
		{`"input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"PRIVATE_THOUGHT"}]}]`, "reasoning_history"},
		{`"input":[{"type":"reasoning","summary":[],"encrypted_content":"PRIVATE_CIPHER"}]`, "encrypted_reasoning_history"},
		{`"input":[{"type":"reasoning","summary":[],"encrypted_content":12}]`, "encrypted_reasoning_history"},
		{`"input":[{"type":"reasoning","summary":[],"private-field":"PRIVATE_VALUE"}]`, "reasoning_history"},
	} {
		t.Run(tc.fields, func(t *testing.T) {
			// Decode fields separately so input overrides the default explicitly.
			m, _ := Decode([]byte(`{` + tc.fields + `}`))
			if _, exists := m["input"]; !exists {
				m["input"] = "hello"
			}
			raw, _ := json.Marshal(m)
			s := newTestSession(t, string(raw), "responses")
			_, err := s.Prepare(AttemptOptions{Protocol: "anthropic"})
			if err == nil || ErrorDiagnostics(err).Category != tc.reason || strings.Contains(err.Error(), "PRIVATE") || strings.Contains(err.Error(), "private-field") {
				t.Fatal(err, ErrorDiagnostics(err))
			}
			if _, err := s.Prepare(AttemptOptions{Protocol: "responses"}); err != nil {
				t.Fatal("changed native preservation", err)
			}
		})
	}
}

func TestAnthropicEgressEmptyOptionsAreFull(t *testing.T) {
	for _, fields := range []string{
		`"client_metadata":{},"stream_options":{},"include":[],"reasoning":{},"text":{}`,
		`"client_metadata":null,"stream_options":null,"include":null,"reasoning":null,"text":null,"truncation":null`,
		`"text":{"format":{"type":"text"}},"truncation":"disabled","input":[{"type":"reasoning","summary":[],"encrypted_content":""},{"role":"user","content":"hello"}]`,
	} {
		m, _ := Decode([]byte(`{` + fields + `}`))
		if m["input"] == nil {
			m["input"] = "hello"
		}
		raw, _ := json.Marshal(m)
		a := prepareTestAttempt(t, newTestSession(t, string(raw), "responses"), "anthropic")
		if a.Diagnostics().Compatibility != Full || len(a.Diagnostics().Degradations) != 0 || strings.Contains(string(a.RequestBody()), `"thinking"`) {
			t.Fatal(string(a.RequestBody()), a.Diagnostics())
		}
	}
}
