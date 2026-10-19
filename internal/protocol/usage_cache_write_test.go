package protocol

import (
	"encoding/json"
	"testing"
)

func TestCacheWriteObservationForReferencePricing(t *testing.T) {
	for _, tc := range []struct {
		protocol, body string
		known          bool
		input, write   int64
	}{
		{"responses", `{"usage":{"input_tokens":100,"output_tokens":10}}`, true, 100, 0},
		{"chat-completions", `{"usage":{"prompt_tokens":100,"completion_tokens":10}}`, true, 100, 0},
		{"anthropic", `{"usage":{"input_tokens":100,"output_tokens":10,"cache_creation_input_tokens":20,"cache_read_input_tokens":30}}`, true, 150, 20},
		{"anthropic", `{"usage":{"input_tokens":100,"output_tokens":10,"cache_creation_input_tokens":0}}`, true, 100, 0},
		{"anthropic", `{"usage":{"input_tokens":100,"output_tokens":10}}`, false, 100, 0},
	} {
		t.Run(tc.protocol+tc.body, func(t *testing.T) {
			var obj Object
			if err := json.Unmarshal([]byte(tc.body), &obj); err != nil {
				t.Fatal(err)
			}
			u := ReadUsage(obj, tc.protocol)
			if !u.Known || u.CacheWriteKnown != tc.known || u.Input != tc.input || u.CacheWrite != tc.write {
				t.Fatal(u)
			}
		})
	}
}
