package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func hasDegradation(d Diagnostics, label Degradation) bool {
	for _, v := range d.Degradations {
		if v == label {
			return true
		}
	}
	return false
}

func TestAnthropicIngressCacheToolErrorsAndParallelPolicy(t *testing.T) {
	raw := `{"model":"public","system":[{"type":"text","text":"system","cache_control":{"type":"ephemeral","ttl":"1h"}}],"metadata":{"user_id":"anonymous"},"tools":[{"type":"custom","name":"lookup","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}}],"tool_choice":{"type":"auto","disable_parallel_tool_use":true},"messages":[{"role":"user","content":[{"type":"text","text":"question","cache_control":{"type":"ephemeral","ttl":"5m"}}]},{"role":"assistant","content":[{"type":"tool_use","id":"c","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"c","is_error":true,"content":[{"type":"text","text":"failure details","cache_control":{"type":"ephemeral"}}]}]}]}`
	s := newTestSession(t, raw, "anthropic")
	if err := s.CheckCapabilities(Capabilities{Streaming: true, Tools: true}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"responses", "chat-completions"} {
		a := prepareTestAttempt(t, s, target)
		m, _ := Decode(a.RequestBody())
		if m["parallel_tool_calls"] != false || m["user"] != "anonymous" || strings.Contains(string(a.body), "cache_control") || !strings.Contains(string(a.body), `[Tool execution failed]\n`) {
			t.Fatalf("%s: %s", target, a.body)
		}
		d := a.Diagnostics()
		if d.Compatibility != Degraded || !hasDegradation(d, PromptCacheControl) || !hasDegradation(d, ToolResultErrorText) {
			t.Fatal(d)
		}
	}
	native := prepareTestAttempt(t, s, "anthropic")
	if !strings.Contains(string(native.body), `"cache_control"`) || !strings.Contains(string(native.body), `"is_error":true`) || native.Diagnostics().Compatibility != Full {
		t.Fatal(string(native.body), native.Diagnostics())
	}
}

func TestAnthropicParallelPolicyRoundtrip(t *testing.T) {
	for _, choice := range []string{`"auto"`, `"required"`, `"none"`, `{"type":"function","name":"lookup"}`} {
		for _, parallel := range []string{"true", "false"} {
			raw := `{"model":"public","input":"hi","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}],"parallel_tool_calls":` + parallel + `,"tool_choice":` + choice + `}`
			wire, err := ConvertRequest([]byte(raw), "responses", "anthropic")
			if err != nil {
				t.Fatal(err)
			}
			m, _ := Decode(wire)
			c := obj(m["tool_choice"])
			if choice == `"none"` {
				if _, exists := c["disable_parallel_tool_use"]; exists {
					t.Fatal(c)
				}
				continue
			}
			if c["disable_parallel_tool_use"] != (parallel == "false") {
				t.Fatal(c)
			}
			back, err := ConvertRequest(wire, "anthropic", "responses")
			if err != nil {
				t.Fatal(err)
			}
			m, _ = Decode(back)
			if m["parallel_tool_calls"] != (parallel == "true") {
				t.Fatal(string(back))
			}
		}
	}
}

func TestAnthropicCrossProtocolRejectsUnknownSemantics(t *testing.T) {
	for _, body := range []string{
		`{"system":[{"type":"text","text":"x","cache_control":{"type":"persistent"}}],"messages":[{"role":"user","content":"x"}]}`,
		`{"messages":[{"role":"user","content":[{"type":"text","text":"x","cache_control":{"type":"ephemeral","ttl":"2h"}}]}]}`,
		`{"metadata":{"private_unknown_field":"secret"},"messages":[{"role":"user","content":"x"}]}`,
		`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"c","is_error":"yes","content":"x"}]}]}`,
		`{"messages":[{"role":"assistant","content":[{"type":"tool_result","tool_use_id":"c","content":"x"}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"tool_use","id":"c","name":"f","input":{}}]}]}`,
		`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"private","signature":"opaque"}]}]}`,
		`{"tool_choice":{"type":"auto","disable_parallel_tool_use":"yes"},"messages":[{"role":"user","content":"x"}]}`,
	} {
		s := newTestSession(t, body, "anthropic")
		for _, target := range []string{"responses", "chat-completions"} {
			if _, err := s.Prepare(AttemptOptions{Protocol: target}); err == nil {
				t.Fatal("accepted", target, body)
			}
		}
		if _, err := s.Prepare(AttemptOptions{Protocol: "anthropic"}); err != nil {
			t.Fatal("native path changed", err)
		}
	}
}

func TestAnthropicMappedToolLoop(t *testing.T) {
	const tools = `[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}},{"type":"custom","name":"patch","format":{"type":"grammar","syntax":"lark","definition":"start: /.+/"}}]},{"type":"function","name":"functions__lookup","parameters":{"type":"object"}}]`
	s := newTestSession(t, `{"input":"hi","tools":`+tools+`,"tool_choice":{"type":"custom","namespace":"functions","name":"patch"}}`, "responses")
	a := prepareTestAttempt(t, s, "anthropic")
	m, _ := Decode(a.body)
	decl := arr(m["tools"])
	lookup, patch, collision := str(obj(decl[0])["name"]), str(obj(decl[1])["name"]), str(obj(decl[2])["name"])
	if lookup == collision || patch != obj(m["tool_choice"])["name"] || !hasDegradation(a.Diagnostics(), GrammarAsDescription) {
		t.Fatal(m)
	}
	input := "line\n雪😀\"\\"
	wrapper, _ := json.Marshal(Object{"input": input})
	var args Object
	_ = json.Unmarshal(wrapper, &args)
	reply, _ := json.Marshal(Object{"id": "r", "stop_reason": "tool_use", "content": []any{Object{"type": "tool_use", "id": "c", "name": patch, "input": args}}, "usage": Object{"input_tokens": 1, "output_tokens": 2}})
	converted, err := a.TranslateResponse(bytes.NewReader(reply))
	if err != nil {
		t.Fatal(err)
	}
	result, _ := Decode(converted.Body)
	call := obj(arr(result["output"])[0])
	if call["type"] != "custom_tool_call" || call["namespace"] != "functions" || call["name"] != "patch" || call["input"] != input || call["call_id"] != "c" {
		t.Fatal(call)
	}
	// A new HTTP request reconstructs the identity without the first attempt.
	history := Object{"input": []any{Object{"role": "user", "content": "hi"}, call, Object{"type": "custom_tool_call_output", "call_id": "c", "output": "done"}}, "tools": arr(mustDecode(t, `{"tools":`+tools+`}`)["tools"])}
	raw, _ := json.Marshal(history)
	next := prepareTestAttempt(t, newTestSession(t, string(raw), "responses"), "anthropic")
	if !strings.Contains(string(next.body), `"tool_use_id":"c"`) || !strings.Contains(string(next.body), `"name":"`+patch+`"`) {
		t.Fatal(string(next.body))
	}
	// An unrelated attempt using the alias as a normal name must stay a function.
	plain := prepareTestAttempt(t, newTestSession(t, `{"input":"hi"}`, "responses"), "anthropic")
	out, err := plain.TranslateResponse(bytes.NewReader(reply))
	if err != nil || strings.Contains(string(out.Body), `custom_tool_call`) {
		t.Fatal(string(out.Body), err)
	}
}

func mustDecode(t *testing.T, raw string) Object {
	t.Helper()
	m, err := Decode([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAnthropicThinkingResponseIsSeparateAndExplicitlyDegraded(t *testing.T) {
	raw := `{"id":"r","content":[{"type":"thinking","thinking":"PRIVATE_THINKING","signature":"PRIVATE_SIGNATURE"},{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`
	for _, client := range []string{"responses", "chat-completions"} {
		a := prepareTestAttempt(t, newTestSession(t, sessionRequests[client], client), "anthropic")
		out, err := a.TranslateResponse(strings.NewReader(raw))
		if err != nil || out.Diagnostics.Compatibility != Degraded || !hasDegradation(out.Diagnostics, AnthropicThinkingPlaintext) || strings.Contains(string(out.Body), "PRIVATE_SIGNATURE") {
			t.Fatal(string(out.Body), out.Diagnostics, err)
		}
		m, _ := Decode(out.Body)
		if client == "responses" {
			if obj(arr(m["output"])[0])["type"] != "reasoning" || obj(arr(m["output"])[1])["type"] != "message" {
				t.Fatal(m)
			}
		} else {
			msg := obj(obj(arr(m["choices"])[0])["message"])
			if msg["reasoning_content"] != "PRIVATE_THINKING" || msg["content"] != "answer" {
				t.Fatal(msg)
			}
		}
	}
	if _, err := ParseResponse([]byte(strings.Replace(raw, `"type":"thinking","thinking":"PRIVATE_THINKING","signature":"PRIVATE_SIGNATURE"`, `"type":"redacted_thinking","data":"opaque"`, 1)), "anthropic"); err == nil {
		t.Fatal("redacted thinking accepted")
	}
	if _, err := ValidateProbeResponse([]byte(raw), "anthropic"); err != nil {
		t.Fatal(err)
	}
}

const anthropicStreamStart = `{"type":"message_start","message":{"id":"r","usage":{"input_tokens":10,"output_tokens":0,"cache_read_input_tokens":4,"cache_creation_input_tokens":2}}}`
const anthropicStreamFinish = `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":10,"output_tokens":3}}`

func TestAnthropicStreamThinkingToolFallbackAndUsage(t *testing.T) {
	for _, client := range []string{"responses", "chat-completions", "anthropic"} {
		a := prepareTestAttempt(t, newTestSession(t, sessionRequests[client], client), "anthropic")
		raw := sse(anthropicStreamStart,
			`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"initial","signature":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" thought"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"PRIVATE_SIGNATURE"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"c","name":"lookup","input":{"city":"雪😀"}}}`,
			`{"type":"content_block_stop","index":1}`, anthropicStreamFinish, `{"type":"message_stop"}`)
		var wire bytes.Buffer
		out, err := a.TranslateStream(context.Background(), &wire, strings.NewReader(raw), StreamOptions{})
		if err != nil || out.Usage.Input != 16 || out.Usage.CacheRead != 4 || out.Usage.CacheWrite != 2 || out.Usage.Output != 3 {
			t.Fatal(client, out.Usage, err)
		}
		if client == "anthropic" {
			if wire.String() != raw || out.Diagnostics.Compatibility != Full {
				t.Fatal("native changed", wire.String())
			}
		} else {
			if strings.Contains(wire.String(), "PRIVATE_SIGNATURE") || !strings.Contains(wire.String(), `city`) || !strings.Contains(wire.String(), `雪😀`) || !hasDegradation(out.Diagnostics, AnthropicThinkingPlaintext) {
				t.Fatal(wire.String(), out.Diagnostics)
			}
			field := "response.reasoning_summary_text.delta"
			if client == "chat-completions" {
				field = "reasoning_content"
			}
			if !strings.Contains(wire.String(), field) {
				t.Fatal(wire.String())
			}
		}
	}
}

func TestAnthropicStreamRejectsBrokenBlockLifecycle(t *testing.T) {
	start := `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"c","name":"lookup","input":{}}}`
	stop := `{"type":"content_block_stop","index":0}`
	for name, middle := range map[string][]string{
		"duplicate_start":     {start, start, stop},
		"wrong_delta_type":    {start, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"bad"}}`, stop},
		"delta_after_stop":    {start, stop, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`},
		"missing_stop":        {start},
		"truncated_arguments": {start, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{"}}`, stop},
		"fractional_index":    {`{"type":"content_block_start","index":0.5,"content_block":{"type":"text","text":"x"}}`},
		"redacted_thinking":   {`{"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"opaque"}}`},
	} {
		t.Run(name, func(t *testing.T) {
			a := prepareTestAttempt(t, newTestSession(t, sessionRequests["responses"], "responses"), "anthropic")
			events := append([]string{anthropicStreamStart}, middle...)
			events = append(events, anthropicStreamFinish, `{"type":"message_stop"}`)
			var wire bytes.Buffer
			_, err := a.TranslateStream(context.Background(), &wire, strings.NewReader(sse(events...)), StreamOptions{})
			if err == nil || strings.Contains(wire.String(), "response.completed") {
				t.Fatal(wire.String(), err)
			}
		})
	}
}

func TestAnthropicCustomStreamFragmentsOverrideStartInput(t *testing.T) {
	a := prepareTestAttempt(t, newTestSession(t, `{"input":"hi","tools":[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"patch"}]}]}`, "responses"), "anthropic")
	input := "line\n雪😀\"\\"
	wrapper, _ := json.Marshal(Object{"input": input})
	cut := strings.Index(string(wrapper), `\n`) + 1
	if cut == 0 {
		t.Fatal("missing split point")
	}
	fragment := func(s string) string {
		b, _ := json.Marshal(Object{"type": "content_block_delta", "index": 0, "delta": Object{"type": "input_json_delta", "partial_json": s}})
		return string(b)
	}
	raw := sse(anthropicStreamStart,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"c","name":"functions__patch","input":{"stale":"fallback"}}}`,
		fragment(string(wrapper[:cut])), fragment(string(wrapper[cut:])),
		`{"type":"content_block_stop","index":0}`, anthropicStreamFinish, `{"type":"message_stop"}`)
	var wire bytes.Buffer
	_, err := a.TranslateStream(context.Background(), &wire, strings.NewReader(raw), StreamOptions{})
	if err != nil || strings.Contains(wire.String(), "fallback") || !strings.Contains(wire.String(), "response.completed") {
		t.Fatal(wire.String(), err)
	}
	for _, line := range strings.Split(wire.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		m, _ := Decode([]byte(strings.TrimPrefix(line, "data: ")))
		if m["type"] == "response.custom_tool_call_input.delta" && m["delta"] != input {
			t.Fatal(m)
		}
		if m["type"] == "response.output_item.done" {
			item := obj(m["item"])
			if item["input"] != input || item["namespace"] != "functions" || item["name"] != "patch" {
				t.Fatal(item)
			}
		}
	}
}

func TestAnthropicUsageDeltaExplicitZerosOverrideStart(t *testing.T) {
	for _, client := range []string{"responses", "anthropic"} {
		a := prepareTestAttempt(t, newTestSession(t, sessionRequests[client], client), "anthropic")
		raw := sse(anthropicStreamStart, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`, `{"type":"message_stop"}`)
		var wire bytes.Buffer
		out, err := a.TranslateStream(context.Background(), &wire, strings.NewReader(raw), StreamOptions{})
		if err != nil || !out.Usage.Known || out.Usage.Input != 0 || out.Usage.Output != 0 || out.Usage.CacheRead != 0 || out.Usage.CacheWrite != 0 {
			t.Fatal(out.Usage, err)
		}
	}
}
