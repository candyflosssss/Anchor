package protocol

import (
	"encoding/json"
	"testing"
)

func TestServiceTierConversion(t *testing.T) {
	for _, tier := range []string{"priority", "ultrafast"} {
		body, _ := json.Marshal(Object{"model": "upstream", "input": "hi", "service_tier": tier})
		chat, err := ConvertRequest(body, "responses", "chat-completions")
		if err != nil {
			t.Fatal(err)
		}
		var got Object
		json.Unmarshal(chat, &got)
		if got["service_tier"] != tier {
			t.Fatal(string(chat))
		}
		response, err := ConvertRequest(chat, "chat-completions", "responses")
		if err != nil {
			t.Fatal(err)
		}
		json.Unmarshal(response, &got)
		if got["service_tier"] != tier {
			t.Fatal(string(response))
		}
		if _, err := ConvertRequest(body, "responses", "anthropic"); err == nil {
			t.Fatal("silently dropped speed policy")
		}
	}
}
