package protocol

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatBridgeCodexRequest(t *testing.T) {
	raw := []byte(`{"model":"deepseek-test","instructions":"system","stream":true,"store":false,"reasoning":{"effort":"high","summary":"auto"},"text":{"format":{"type":"json_schema","name":"reply","strict":true,"schema":{"type":"object"}}},"include":["reasoning.encrypted_content"],"prompt_cache_key":"synthetic-cache","metadata":{"purpose":"test"},"truncation":"disabled","parallel_tool_calls":true,"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]},{"type":"reasoning","summary":[{"type":"summary_text","text":"check"}]},{"type":"function_call","namespace":"functions","call_id":"a","name":"lookup","arguments":"{}"},{"type":"custom_tool_call","call_id":"b","name":"patch","input":"line\nsecond"},{"type":"function_call_output","call_id":"a","output":"one"},{"type":"custom_tool_call_output","call_id":"b","output":[{"type":"input_text","text":"two"}]}],"tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]},{"type":"custom","name":"patch","format":{"type":"grammar","syntax":"lark","definition":"start: /.+/"}}]}`)
	body, mapping, err := ConvertRequestMapped(raw, "responses", "chat-completions")
	if err != nil {
		t.Fatal(err)
	}
	m, _ := Decode(body)
	if m["reasoning_effort"] != "high" || m["reasoning"] != nil || m["include"] != nil || m["text"] != nil || m["prompt_cache_key"] != "synthetic-cache" || obj(m["stream_options"])["include_usage"] != true {
		t.Fatalf("options: %s", body)
	}
	if obj(m["response_format"])["type"] != "json_schema" || obj(obj(m["response_format"])["json_schema"])["name"] != "reply" {
		t.Fatalf("format: %s", body)
	}
	messages := arr(m["messages"])
	if len(messages) != 5 || obj(messages[2])["reasoning_content"] != "check" || len(arr(obj(messages[2])["tool_calls"])) != 2 || obj(messages[3])["tool_call_id"] != "a" || obj(messages[4])["content"] != "two" {
		t.Fatalf("history: %s", body)
	}
	tools := arr(m["tools"])
	if len(tools) != 2 || !strings.Contains(str(obj(obj(tools[1])["function"])["description"]), "start: /.+/") {
		t.Fatalf("tools: %s", body)
	}
	custom := Item{Tool: true, Name: str(obj(obj(tools[1])["function"])["name"]), Arguments: `{"input":"line\nsecond"}`}
	r := Result{ID: "r", Items: []Item{custom}}
	if err := mapping.RestoreResult(&r); err != nil || !r.Items[0].Custom || r.item(0)["input"] != "line\nsecond" {
		t.Fatalf("restore: %+v %v", r, err)
	}
	// Chat ingress retains the same reasoning and structured-output settings.
	back, err := ConvertRequest(body, "chat-completions", "responses")
	if err != nil || !strings.Contains(string(back), `"effort":"high"`) || !strings.Contains(string(back), `"type":"reasoning"`) || !strings.Contains(string(back), `"type":"json_schema"`) {
		t.Fatalf("ingress: %s %v", back, err)
	}
}

func TestChatBridgeFormatsAndInlineTools(t *testing.T) {
	for _, format := range []string{`{"type":"text"}`, `{"type":"json_object"}`} {
		body, err := ConvertRequest([]byte(`{"model":"m","input":"hi","text":{"format":`+format+`,"verbosity":"low"}}`), "responses", "chat-completions")
		if err != nil || !strings.Contains(string(body), `"response_format":`+format) || !strings.Contains(string(body), `"verbosity":"low"`) {
			t.Fatalf("%s %v", body, err)
		}
	}
	body, _, err := ConvertRequestMapped([]byte(`{"model":"m","input":[{"type":"additional_tools","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]},{"role":"user","content":"hi"}]}`), "responses", "chat-completions")
	if err != nil || !strings.Contains(string(body), `"name":"lookup"`) || strings.Contains(string(body), "additional_tools") {
		t.Fatalf("%s %v", body, err)
	}
	for _, schema := range []string{`{}`, `{"type":"object"}`} {
		body, err := ConvertRequest([]byte(`{"model":"m","tools":[{"type":"function","name":"lookup","parameters":{}}],"input":[{"role":"user","content":"hi"},{"type":"additional_tools","tools":[{"type":"function","name":"lookup","parameters":`+schema+`}]}]}`), "responses", "chat-completions")
		if schema == `{}` {
			if err != nil || strings.Count(string(body), `"name":"lookup"`) != 1 {
				t.Fatalf("duplicate declarations: %s %v", body, err)
			}
		} else if err == nil {
			t.Fatal("conflicting duplicate schema silently accepted")
		}
	}
}

func TestChatBridgeConversionErrorsAreSafe(t *testing.T) {
	for _, tc := range []struct{ fields, reason string }{
		{`"reasoning":{"private-field":"private-value"}`, "reasoning_options"},
		{`"reasoning":{"effort":12}`, "reasoning_options"},
		{`"reasoning":{"effort":"private-effort"}`, "reasoning_options"},
		{`"reasoning":{"effort":"low medium"}`, "reasoning_options"},
		{`"text":{"format":{"type":"private-format"}}`, "text_format"},
		{`"text":{"format":{"type":"json_schema","name":"n"}}`, "text_format"},
		{`"include":["private-include"]`, "include_options"},
		{`"include":{}`, "include_options"},
		{`"truncation":"auto"`, "truncation"},
		{`"private-field":"private-value"`, "request_fields"},
		{`"tools":[{"type":"web_search"}]`, "tools"},
		{`"tools":[{"type":"tool_search","execution":"client"}]`, "tools"},
		{`"input":[{"type":"function_call","call_id":"c","name":"lookup","namespace":{},"arguments":"{}"}]`, "tools"},
		{`"tools":[{"type":"custom","name":"f","format":{"type":"grammar","syntax":"private-syntax","definition":"private-grammar"}}]`, "custom_tool_grammar"},
		{`"input":[{"type":"reasoning","encrypted_content":"private-ciphertext","summary":[]}]`, "encrypted_reasoning_history"},
		{`"input":[{"role":"user","content":[{"type":"input_image","image_url":"private-image"}]}]`, "request_content"},
	} {
		// Build the object without duplicate input keys.
		var fields Object
		if err := json.Unmarshal([]byte(`{`+tc.fields+`}`), &fields); err != nil {
			t.Fatal(err)
		}
		fields["model"] = "m"
		if _, exists := fields["input"]; !exists {
			fields["input"] = "hi"
		}
		raw, _ := json.Marshal(fields)
		_, _, err := ConvertRequestMapped(raw, "responses", "chat-completions")
		if !errors.Is(err, ErrUnsupported) || ConversionReason(err) != tc.reason || strings.Contains(err.Error(), "private-") {
			t.Fatalf("%s: reason=%s error=%v", tc.fields, ConversionReason(err), err)
		}
	}
	if ConversionReason(errors.New("private-error")) != "request_content" {
		t.Fatal("unsafe fallback")
	}
}

func TestChatBridgeToolIdentityCollisions(t *testing.T) {
	request := Object{"model": "m", "input": "hi", "tools": []any{
		Object{"type": "function", "name": "a__lookup", "parameters": Object{}},
		Object{"type": "namespace", "name": "a", "tools": []any{Object{"type": "function", "name": "lookup", "parameters": Object{}}}},
		Object{"type": "custom", "name": strings.Repeat("long_", 25)},
	}}
	raw, _ := json.Marshal(request)
	body, mapping, err := ConvertRequestMapped(raw, "responses", "chat-completions")
	if err != nil {
		t.Fatal(err)
	}
	m, _ := Decode(body)
	seen := map[string]bool{}
	for i, value := range arr(m["tools"]) {
		name := str(obj(obj(value)["function"])["name"])
		if name == "" || len(name) > 64 || seen[name] {
			t.Fatalf("alias %q", name)
		}
		seen[name] = true
		item := Item{Tool: true, Name: name}
		mapping.restore(&item)
		switch i {
		case 0:
			if item.Name != "a__lookup" || item.Namespace != "" {
				t.Fatal(item)
			}
		case 1:
			if item.Name != "lookup" || item.Namespace != "a" {
				t.Fatal(item)
			}
		case 2:
			if item.Name != strings.Repeat("long_", 25) || !item.Custom {
				t.Fatal(item)
			}
		}
	}
	// A second request with the same wire alias has no access to the first map.
	_, other, err := ConvertRequestMapped([]byte(`{"model":"m","input":"hi","tools":[{"type":"function","name":"a__lookup","parameters":{}}]}`), "responses", "chat-completions")
	if err != nil {
		t.Fatal(err)
	}
	item := Item{Tool: true, Name: "a__lookup"}
	other.restore(&item)
	if item.Custom || item.Namespace != "" {
		t.Fatal("mapping leaked across requests", item)
	}
}

func TestChatBridgeNamespaceChoiceAndHistory(t *testing.T) {
	for _, namespace := range []string{"", `,"namespace":"functions"`} {
		raw := []byte(`{"model":"m","input":[{"type":"function_call","call_id":"c","name":"lookup","arguments":"{}"` + namespace + `}],"tool_choice":{"type":"function","name":"lookup"` + namespace + `},"tools":[{"type":"namespace","name":"functions","tools":[{"type":"function","name":"lookup","parameters":{}}]}]}`)
		body, _, err := ConvertRequestMapped(raw, "responses", "chat-completions")
		if err != nil {
			t.Fatal(err)
		}
		m, _ := Decode(body)
		if obj(obj(m["tool_choice"])["function"])["name"] != "functions__lookup" || !strings.Contains(string(body), `"name":"functions__lookup"`) {
			t.Fatal(string(body))
		}
	}
	for _, choice := range []string{`{"type":"function","name":"missing"}`, `{"type":"function","name":"lookup"}`} {
		raw := []byte(`{"model":"m","input":"hi","tool_choice":` + choice + `,"tools":[{"type":"namespace","name":"a","tools":[{"type":"function","name":"lookup","parameters":{}}]},{"type":"namespace","name":"b","tools":[{"type":"function","name":"lookup","parameters":{}}]}]}`)
		if _, err := ConvertRequest(raw, "responses", "chat-completions"); err == nil {
			t.Fatal("guessed undeclared or ambiguous choice", choice)
		}
	}
}

func bridgeEvents(t *testing.T, body string) []Object {
	t.Helper()
	var result []Object
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		m, err := Decode([]byte(strings.TrimPrefix(line, "data: ")))
		if err != nil {
			t.Fatal(err, body)
		}
		result = append(result, m)
	}
	return result
}

func TestChatBridgeReasoningAndCustomToolStream(t *testing.T) {
	_, mapping, err := ConvertRequestMapped([]byte(`{"model":"m","input":"hi","tools":[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"patch"},{"type":"function","name":"lookup","parameters":{}}]}]}`), "responses", "chat-completions")
	if err != nil {
		t.Fatal(err)
	}
	customAlias := mapping.byIdentity[toolIdentity{"functions", "patch", true}]
	functionAlias := mapping.byIdentity[toolIdentity{"functions", "lookup", false}]
	arguments, _ := json.Marshal(Object{"input": "line\n\"你好\"\\tail"})
	chunk := func(delta Object) string {
		b, _ := json.Marshal(Object{"id": "r", "choices": []any{Object{"index": 0, "delta": delta}}})
		return string(b)
	}
	chunks := []string{
		chunk(Object{"role": "assistant", "reasoning_content": "think"}),
		chunk(Object{"content": "reply"}),
		chunk(Object{"tool_calls": []any{Object{"index": 0, "id": "custom", "type": "function", "function": Object{"name": customAlias, "arguments": ""}}, Object{"index": 1, "id": "fn", "type": "function", "function": Object{"name": functionAlias, "arguments": "{}"}}}}),
	}
	// Split at every rune, including inside JSON escapes. Each SSE payload
	// stays valid JSON while the accumulated argument string is incomplete.
	for _, ch := range string(arguments) {
		chunks = append(chunks, chunk(Object{"tool_calls": []any{Object{"index": 0, "function": Object{"arguments": string(ch)}}}}))
	}
	chunks = append(chunks, `{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`, `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":7,"completion_tokens_details":{"reasoning_tokens":3}}}`, "[DONE]")
	w := httptest.NewRecorder()
	observation := &Observation{}
	u, err := StreamObservedMapped(w, strings.NewReader(sse(chunks...)), "chat-completions", "responses", "public", observation, mapping)
	if err != nil || !u.Known || !u.ReasoningKnown || u.Reasoning != 3 || !observation.Reasoning || observation.FirstText.IsZero() {
		t.Fatalf("usage=%+v obs=%+v err=%v\n%s", u, observation, err, w.Body.String())
	}
	var terminal Object
	var reasoning, text, custom string
	for i, event := range bridgeEvents(t, w.Body.String()) {
		if num(event["sequence_number"]) != int64(i) {
			t.Fatal("non-monotonic sequence", event)
		}
		switch event["type"] {
		case "response.reasoning_summary_text.delta":
			reasoning += str(event["delta"])
		case "response.output_text.delta":
			text += str(event["delta"])
		case "response.custom_tool_call_input.delta":
			custom += str(event["delta"])
		case "response.completed":
			terminal = obj(event["response"])
		}
	}
	if reasoning != "think" || text != "reply" || custom != "line\n\"你好\"\\tail" || terminal == nil {
		t.Fatal(reasoning, text, custom, terminal)
	}
	items := arr(terminal["output"])
	if len(items) != 4 || obj(items[0])["type"] != "reasoning" || obj(items[2])["type"] != "custom_tool_call" || obj(items[2])["namespace"] != "functions" || obj(items[2])["name"] != "patch" || obj(items[3])["name"] != "lookup" || obj(obj(terminal["usage"])["output_tokens_details"])["reasoning_tokens"] != float64(3) {
		t.Fatal(terminal)
	}
	// Replaying the generated items into the next request preserves reasoning
	// and custom-tool input rather than turning either into visible user text.
	replay := Object{"model": "m", "input": items}
	raw, _ := json.Marshal(replay)
	body, err := ConvertRequest(raw, "responses", "chat-completions")
	if err != nil || !strings.Contains(string(body), `"reasoning_content":"think"`) {
		t.Fatalf("replay: %s %v", body, err)
	}
}

func TestChatBridgeStreamDoesNotCompleteOnInvalidCustomArguments(t *testing.T) {
	_, mapping, err := ConvertRequestMapped([]byte(`{"model":"m","input":"hi","tools":[{"type":"custom","name":"patch"}]}`), "responses", "chat-completions")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{`{"input":`, `{"input":42}`, `{"input":"x","extra":"y"}`} {
		chunk, _ := json.Marshal(Object{"choices": []any{Object{"index": 0, "delta": Object{"tool_calls": []any{Object{"index": 0, "id": "c", "function": Object{"name": "patch", "arguments": args}}}}}}})
		w := httptest.NewRecorder()
		_, err := StreamObservedMapped(w, strings.NewReader(sse(string(chunk), "[DONE]")), "chat-completions", "responses", "public", nil, mapping)
		if err == nil || strings.Contains(w.Body.String(), "response.completed") || strings.Contains(w.Body.String(), "response.custom_tool_call_input.delta") || !strings.Contains(w.Body.String(), "upstream_stream_error") {
			t.Fatalf("args=%s err=%v %s", args, err, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	_, err = StreamObservedMapped(w, strings.NewReader(sse(`{"choices":[{"index":0,"delta":{"reasoning_content":"think"}}]}`)), "chat-completions", "responses", "public", nil, mapping)
	if err == nil || strings.Contains(w.Body.String(), "response.completed") {
		t.Fatal("truncated reasoning stream completed", err, w.Body.String())
	}
}

func TestChatBridgeNonStreamingReasoning(t *testing.T) {
	r, err := ParseResponse([]byte(`{"id":"r","choices":[{"message":{"reasoning_content":"think","content":"reply"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"completion_tokens_details":{"reasoning_tokens":1}}}`), "chat-completions")
	if err != nil || len(r.Items) != 2 || r.CheckTarget("anthropic") == nil {
		t.Fatal(r, err)
	}
	output := r.JSON("responses")
	if obj(arr(output["output"])[0])["type"] != "reasoning" || obj(arr(output["output"])[1])["type"] != "message" {
		t.Fatal(output)
	}
	probe, err := ParseProbeResponse([]byte(`{"id":"r","choices":[{"message":{"reasoning_content":"think","content":"reply"}}]}`), "chat-completions")
	if err != nil || len(probe.Items) != 1 || probe.Items[0].Reasoning {
		t.Fatal("probe leaked reasoning into visible reply", probe, err)
	}
}
