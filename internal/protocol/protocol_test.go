package protocol

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPriorityProtocolRequestsAndTools(t *testing.T) {
	raw := []byte(`{"model":"up","instructions":"system","input":[{"role":"user","content":"hello"},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"x\":1}"},{"type":"function_call_output","call_id":"call_1","output":"ok"}],"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}],"stream":true,"store":false}`)
	for _, p := range []string{"chat-completions", "anthropic"} {
		converted, err := ConvertRequest(raw, "responses", p)
		if err != nil {
			t.Fatal(p, err)
		}
		back, err := ConvertRequest(converted, p, "responses")
		if err != nil {
			t.Fatal(p, err, string(converted))
		}
		m, _ := Decode(back)
		if len(arr(m["input"])) < 3 || !strings.Contains(string(back), `"call_id":"call_1"`) || !strings.Contains(string(back), `"function_call_output"`) {
			t.Fatal(string(back))
		}
	}
	for _, body := range []string{
		`{"model":"m","input":"hi","tools":{}}`,
		`{"model":"m","input":[{"role":"user","content":[{"type":"input_image","image_url":"x"}]}]}`,
		`{"model":"m","input":"hi","reasoning":{"effort":"high"}}`,
		`{"model":"m","input":"hi","tool_choice":{"type":"function","name":"f","extra":1}}`,
	} {
		if _, err := ConvertRequest([]byte(body), "responses", "anthropic"); err == nil {
			t.Fatal("unsupported conversion accepted", body)
		}
	}
}
func TestPriorityProtocolResponsesAndUnknownUsage(t *testing.T) {
	for p, raw := range map[string]string{
		"responses":        `{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]},{"type":"function_call","call_id":"c","name":"f","arguments":"{}"}],"usage":{"input_tokens":10,"output_tokens":3,"input_tokens_details":{"cached_tokens":4}}}`,
		"chat-completions": `{"id":"r","choices":[{"finish_reason":"tool_calls","message":{"content":"ok","tool_calls":[{"id":"c","type":"function","function":{"name":"f","arguments":"{}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":4}}}`,
		"anthropic":        `{"id":"r","stop_reason":"tool_use","content":[{"type":"text","text":"ok"},{"type":"tool_use","id":"c","name":"f","input":{}}],"usage":{"input_tokens":6,"cache_read_input_tokens":4,"output_tokens":3}}`,
	} {
		r, err := ParseResponse([]byte(raw), p)
		if err != nil || !r.Usage.Known || r.Usage.Input != 10 || r.Usage.CacheRead != 4 {
			t.Fatal(p, r, err)
		}
		for _, dest := range []string{"responses", "chat-completions", "anthropic"} {
			b, _ := json.Marshal(r.JSON(dest))
			round, err := ParseResponse(b, dest)
			if err != nil || len(round.Items) != 2 || round.Items[1].Arguments != "{}" {
				t.Fatal(dest, string(b), err)
			}
		}
	}
	for _, raw := range []string{`{}`, `{"usage":{"input_tokens":null,"output_tokens":0}}`, `{"usage":{"input_tokens":-1,"output_tokens":2}}`} {
		m, _ := Decode([]byte(raw))
		if ReadUsage(m, "responses").Known {
			t.Fatal("invented usage")
		}
	}
}
func sse(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		fmt.Fprintf(&b, "data: %s\n\n", e)
	}
	return b.String()
}
func TestPriorityProtocolStreams(t *testing.T) {
	streams := map[string]string{
		"chat-completions": sse(`{"id":"c","choices":[{"index":0,"delta":{"content":"hi"}}]}`, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call","function":{"name":"f","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2}}`, `[DONE]`),
		"anthropic":        sse(`{"type":"message_start","message":{"id":"c","usage":{"input_tokens":10,"output_tokens":0}}}`, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`, `{"type":"content_block_stop","index":0}`, `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call","name":"f","input":{}}}`, `{"type":"content_block_stop","index":1}`, `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":2}}`, `{"type":"message_stop"}`),
		"responses":        sse(`{"type":"response.created","response":{"id":"c"}}`, `{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg"}}`, `{"type":"response.output_text.delta","output_index":0,"delta":"hi"}`, `{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fn","call_id":"call","name":"f"}}`, `{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{}"}`, `{"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":2}}}`),
	}
	for from, data := range streams {
		for _, to := range []string{"responses", "chat-completions", "anthropic"} {
			t.Run(from+"/"+to, func(t *testing.T) {
				w := httptest.NewRecorder()
				u, e := Stream(w, strings.NewReader(data), from, to, "public")
				if e != nil || !u.Known || u.Input != 10 || u.Output != 2 {
					t.Fatal(e, u, w.Body.String())
				}
				if !strings.Contains(w.Body.String(), "hi") {
					t.Fatal("text lost")
				}
				if from != to {
					copy := httptest.NewRecorder()
					u, e = Stream(copy, strings.NewReader(w.Body.String()), to, to, "public")
					if e != nil || !u.Known || u.Input != 10 {
						t.Fatal("emitted stream invalid", e, u, w.Body.String())
					}
				}
			})
		}
	}
	w := httptest.NewRecorder()
	_, err := Stream(w, strings.NewReader(sse(`{"choices":[{"index":0,"delta":{"content":"partial"}}]}`)), "chat-completions", "responses", "public")
	if err == nil || strings.Contains(w.Body.String(), "response.completed") || !strings.Contains(w.Body.String(), "upstream_stream_error") {
		t.Fatal("truncated stream completed", w.Body.String())
	}
}

func TestPriorityParallelToolsAndUnsupportedStream(t *testing.T) {
	raw := []byte(`{"model":"up","input":[{"role":"assistant","content":"Checking"},{"type":"function_call","call_id":"a","name":"lookup","arguments":"{}"},{"type":"function_call","call_id":"b","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"a","output":"one"},{"type":"function_call_output","call_id":"b","output":"two"}]}`)
	converted, err := ConvertRequest(raw, "responses", "chat-completions")
	if err != nil {
		t.Fatal(err)
	}
	m, _ := Decode(converted)
	messages := arr(m["messages"])
	if len(messages) != 3 || len(arr(obj(messages[0])["tool_calls"])) != 2 || obj(messages[0])["content"] != "Checking" {
		t.Fatal(string(converted))
	}
	for _, tc := range []struct{ from, payload string }{
		{"responses", `{"type":"response.output_text.annotation.added","annotation":{"type":"url_citation"}}`},
		{"responses", `{"type":"response.reasoning_summary_text.delta","delta":"reasoning"}`},
		{"anthropic", `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"reasoning"}}`},
		{"chat-completions", `{"choices":[{"index":0,"delta":{"audio":{"data":"unsupported"}}}]}`},
	} {
		w := httptest.NewRecorder()
		target := "responses"
		if tc.from == target {
			target = "chat-completions"
		}
		_, err := Stream(w, strings.NewReader("data: "+tc.payload+"\n\n"), tc.from, target, "public")
		if err == nil || strings.Contains(w.Body.String(), "response.completed") {
			t.Fatal(tc.from, err, w.Body.String())
		}
	}
}

func TestPriorityStreamParallelTools(t *testing.T) {
	data := `data: {"id":"chat-id","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"a","type":"function","function":{"name":"lookup","arguments":""}},{"index":1,"id":"b","type":"function","function":{"name":"lookup","arguments":""}}]}}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"x\":2}"}},{"index":0,"function":{"arguments":"{\"x\":1}"}}]}}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":4}}

data: [DONE]

`
	for _, target := range []string{"responses", "anthropic"} {
		w := httptest.NewRecorder()
		u, err := Stream(w, strings.NewReader(data), "chat-completions", target, "public")
		if err != nil || !u.Known {
			t.Fatal(target, err, w.Body.String())
		}
		out := httptest.NewRecorder()
		_, err = Stream(out, strings.NewReader(w.Body.String()), target, "chat-completions", "public")
		if err != nil || !strings.Contains(out.Body.String(), `"id":"a"`) || !strings.Contains(out.Body.String(), `"id":"b"`) || !strings.Contains(out.Body.String(), `"finish_reason":"tool_calls"`) {
			t.Fatal(target, err, out.Body.String())
		}
	}
}
