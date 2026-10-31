package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

const syntheticPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j1XkAAAAASUVORK5CYII="

func TestAnthropicImageInputAndToolResultPreserved(t *testing.T) {
	image := Object{"type": "input_image", "image_url": "data:image/png;base64," + syntheticPNG, "detail": "high"}
	content := []any{Object{"type": "input_text", "text": "BEFORE"}, image, Object{"type": "input_text", "text": "AFTER"}}
	for _, toolResult := range []bool{false, true} {
		input := []any{Object{"role": "user", "content": content}}
		if toolResult {
			input = []any{Object{"role": "user", "content": "question"}, Object{"type": "function_call", "call_id": "c", "name": "lookup", "arguments": "{}"}, Object{"type": "function_call_output", "call_id": "c", "output": content}}
		}
		raw, _ := json.Marshal(Object{"model": "public", "input": input})
		s := newTestSession(t, string(raw), "responses")
		a := prepareTestAttempt(t, s, "anthropic")
		wire, _ := Decode(a.RequestBody())
		messages := arr(wire["messages"])
		blocks := arr(obj(messages[len(messages)-1])["content"])
		if toolResult {
			result := obj(blocks[0])
			if result["tool_use_id"] != "c" {
				t.Fatal(result)
			}
			blocks = arr(result["content"])
		}
		if len(blocks) != 3 || obj(blocks[0])["text"] != "BEFORE" || obj(blocks[2])["text"] != "AFTER" {
			t.Fatal("ordered content lost")
		}
		source := obj(obj(blocks[1])["source"])
		if obj(blocks[1])["type"] != "image" || source["type"] != "base64" || source["media_type"] != "image/png" || source["data"] != syntheticPNG {
			t.Fatal("image changed")
		}
		if !hasDegradation(a.Diagnostics(), "image_detail_omitted") {
			t.Fatal(a.Diagnostics())
		}
		native := prepareTestAttempt(t, s, "responses")
		preserved, _ := Decode(native.RequestBody())
		before, _ := json.Marshal(input)
		after, _ := json.Marshal(preserved["input"])
		if string(before) != string(after) {
			t.Fatal("native images changed")
		}
	}
}

func TestAnthropicImageURLPreservedWithoutFetching(t *testing.T) {
	a := prepareTestAttempt(t, newTestSession(t, `{"input":[{"role":"user","content":[{"type":"input_image","image_url":"https://example.invalid/fixture.png","detail":"auto"}]}]}`, "responses"), "anthropic")
	if !strings.Contains(string(a.RequestBody()), `"source":{"type":"url","url":"https://example.invalid/fixture.png"}`) || a.Diagnostics().Compatibility != Full {
		t.Fatal(a.Diagnostics())
	}
}

func TestAnthropicRejectsUnsupportedImageAndTextMetadata(t *testing.T) {
	for _, part := range []string{
		`{"type":"input_image","file_id":"PRIVATE_FILE"}`,
		`{"type":"input_image","image_url":"data:image/svg+xml;base64,YQ=="}`,
		`{"type":"input_image","image_url":"data:image/png;base64,not-base64"}`,
		`{"type":"input_image","image_url":"file:///PRIVATE_PATH"}`,
		`{"type":"input_image","image_url":"https://example.invalid/a","detail":"PRIVATE_DETAIL"}`,
		`{"type":"output_text","text":"hello","logprobs":[{}]}`,
		`{"type":"output_text","text":"hello","logprobs":{}}`,
		`{"type":"output_text","text":"hello","annotations":[{}]}`,
	} {
		s := newTestSession(t, `{"input":[{"role":"user","content":[`+part+`]}]}`, "responses")
		_, err := s.Prepare(AttemptOptions{Protocol: "anthropic"})
		if err == nil || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("unsafe image/metadata acceptance", err)
		}
	}
	for _, metadata := range []string{`"logprobs":[],"annotations":[]`, `"logprobs":null,"annotations":null`} {
		a := prepareTestAttempt(t, newTestSession(t, `{"input":[{"role":"assistant","content":[{"type":"output_text","text":"VISIBLE",`+metadata+`}]}]}`, "responses"), "anthropic")
		if !strings.Contains(string(a.RequestBody()), "VISIBLE") || a.Diagnostics().Compatibility != Full {
			t.Fatal(a.Diagnostics())
		}
	}
}
