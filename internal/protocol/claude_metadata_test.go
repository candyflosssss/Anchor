package protocol

import "testing"

func TestAnthropicMetadataNativeOnlyAndIsolation(t *testing.T) {
	for from, raw := range sessionRequests {
		for to := range sessionRequests {
			s := newTestSession(t, raw, from)
			metadata := RequestMetadata{AnthropicVersion: "2023-06-01", AnthropicBeta: []string{"first-beta", "second-beta"}}
			a, err := s.Prepare(AttemptOptions{Protocol: to, Model: "selected", ClientMetadata: metadata})
			if err != nil {
				t.Fatal(err)
			}
			headers := a.RequestHeaders()
			if from == "anthropic" && to == "anthropic" {
				if len(headers) != 3 || headers[0].Value != "2023-06-01" || headers[1].Value != "first-beta" || headers[2].Value != "second-beta" {
					t.Fatal(headers)
				}
				headers[0].Value = "mutated"
				metadata.AnthropicBeta[0] = "mutated"
				if a.RequestHeaders()[0].Value != "2023-06-01" || a.RequestHeaders()[1].Value != "first-beta" {
					t.Fatal("metadata aliases caller memory")
				}
			} else if len(headers) != 0 {
				t.Fatalf("metadata leaked across protocols %s -> %s", from, to)
			}
			other, err := s.Prepare(AttemptOptions{Protocol: to, Model: "other"})
			if err != nil || len(other.RequestHeaders()) != 0 {
				t.Fatal("metadata leaked to another attempt", err)
			}
		}
	}
}
