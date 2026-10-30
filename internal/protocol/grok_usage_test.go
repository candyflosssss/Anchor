package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func grokUsageFixture(p string, input, output, reasoning int64, total any) Object {
	in, out, inputDetails, outputDetails := "input_tokens", "output_tokens", "input_tokens_details", "output_tokens_details"
	if p == "chat-completions" {
		in, out, inputDetails, outputDetails = "prompt_tokens", "completion_tokens", "prompt_tokens_details", "completion_tokens_details"
	}
	u := Object{in: input, out: output, inputDetails: Object{"cached_tokens": 4}, outputDetails: Object{"reasoning_tokens": reasoning}}
	if total != nil {
		u["total_tokens"] = total
	}
	// Match Decode's JSON number representation, not hand-constructed Go ints.
	b, _ := json.Marshal(Object{"usage": u})
	m, _ := Decode(b)
	return m
}

func TestGrokIndependentReasoningUsage(t *testing.T) {
	for _, p := range []string{"responses", "chat-completions"} {
		for _, tc := range []struct {
			name              string
			output, reasoning int64
			total             any
			known             bool
			want              int64
		}{
			{"xai_chat_example", 9, 94, 135, true, 103},
			{"xai_responses_example", 9, 110, 151, true, 119},
			{"independent_smaller_than_visible", 20, 5, 57, true, 25},
			{"independent_reasoning_only", 0, 94, 126, true, 94},
			{"inclusive_openai", 103, 94, 135, true, 103},
			{"inclusive_without_total", 103, 94, nil, true, 103},
			{"inclusive_small_reasoning", 20, 5, 52, true, 20},
			{"explicit_zero", 0, 0, 32, true, 0},
			{"maximum_valid_total", 9, 999999999959, 1e12, true, 999999999968},
			{"partial_gap_preserves_existing_usage", 20, 5, 55, true, 20},
			{"bad_total_preserves_existing_usage", 20, 5, 52.5, true, 20},
			{"missing_total_no_guess", 9, 94, nil, false, 0},
			{"null_total_no_guess", 9, 94, json.RawMessage("null"), false, 0},
			{"string_total_no_coercion", 9, 94, "135", false, 0},
			{"fractional_total", 9, 94, 135.5, false, 0},
			{"negative_total", 9, 94, -135, false, 0},
			{"oversized_total", 9, 94, 1e12 + 1, false, 0},
			{"partial_gap_no_guess", 9, 94, 134, false, 0},
			{"larger_gap_no_guess", 9, 94, 136, false, 0},
			{"inclusive_but_impossible_reasoning", 9, 94, 41, false, 0},
		} {
			t.Run(p+"/"+tc.name, func(t *testing.T) {
				m := grokUsageFixture(p, 32, tc.output, tc.reasoning, tc.total)
				before, _ := json.Marshal(m)
				u := ReadUsage(m, p)
				after, _ := json.Marshal(m)
				if !bytes.Equal(before, after) {
					t.Fatal("usage normalization modified native payload")
				}
				if u.Known != tc.known || u.Output != tc.want {
					t.Fatalf("got %+v; want known=%t output=%d", u, tc.known, tc.want)
				}
				if !tc.known {
					return
				}
				if u.Input != 32 || !u.ReasoningKnown || u.Reasoning != tc.reasoning || !u.CacheReadKnown || u.CacheRead != 4 || !u.CacheWriteKnown {
					t.Fatal("lost usage detail", u)
				}
				for _, target := range []string{"responses", "chat-completions", "anthropic"} {
					b, _ := json.Marshal(Object{"usage": u.JSON(target)})
					roundTrip, _ := Decode(b)
					next := ReadUsage(roundTrip, target)
					if !next.Known || next.Input != u.Input || next.Output != u.Output || next.CacheRead != u.CacheRead || target != "anthropic" && next.Reasoning != u.Reasoning {
						t.Fatalf("%s double counted or lost usage: %+v -> %+v", target, u, next)
					}
				}
			})
		}
	}
}

func TestGrokUsageTranslationMatrix(t *testing.T) {
	for _, upstream := range []string{"responses", "chat-completions"} {
		usage, _ := json.Marshal(obj(grokUsageFixture(upstream, 32, 9, 94, 135)["usage"]))
		for _, client := range []string{"responses", "chat-completions", "anthropic"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s_to_%s/stream_%t", upstream, client, stream), func(t *testing.T) {
					a := prepareTestAttempt(t, newTestSession(t, sessionRequests[client], client), upstream)
					original := sessionReplies[upstream]
					if stream {
						original = sessionStreams[upstream]
					}
					oldUsage := map[string]string{"responses": `{"input_tokens":0,"output_tokens":0}`, "chat-completions": `{"prompt_tokens":0,"completion_tokens":0}`}[upstream]
					wire := strings.Replace(original, `"usage":`+oldUsage, `"usage":`+string(usage), 1)
					if wire == original {
						t.Fatal("fixture did not replace usage")
					}
					if !stream {
						u, err := ValidateProbeResponse([]byte(wire), upstream)
						if err != nil || !u.Known || u.Output != 103 || u.Reasoning != 94 {
							t.Fatalf("probe usage=%+v err=%v", u, err)
						}
					}
					var result Response
					var body bytes.Buffer
					var err error
					if stream {
						result, err = a.TranslateStream(t.Context(), &body, strings.NewReader(wire), StreamOptions{})
					} else {
						result, err = a.TranslateResponse(strings.NewReader(wire))
						body.Write(result.Body)
					}
					if err != nil || !result.Usage.Known || result.Usage.Input != 32 || result.Usage.Output != 103 || !result.Usage.ReasoningKnown || result.Usage.Reasoning != 94 {
						t.Fatalf("usage=%+v diagnostics=%+v err=%v body=%s", result.Usage, result.Diagnostics, err, body.String())
					}
					if upstream == client {
						if body.String() != wire {
							t.Fatal("native response was re-encoded", body.String())
						}
						return
					}
					if !strings.Contains(body.String(), "hello") || strings.Contains(body.String(), "event: error") {
						t.Fatal("translation lost reply or failed", body.String())
					}
					if !stream {
						m, _ := Decode(body.Bytes())
						checkGrokCanonicalUsage(t, m, client)
					} else {
						found := false
						for _, line := range strings.Split(body.String(), "\n") {
							if !strings.HasPrefix(line, "data: {") {
								continue
							}
							m, _ := Decode([]byte(strings.TrimPrefix(line, "data: ")))
							if client == "responses" {
								m = obj(m["response"])
							}
							if ReadUsage(m, client).Known {
								checkGrokCanonicalUsage(t, m, client)
								found = true
							}
						}
						if !found {
							t.Fatal("no canonical usage in stream", body.String())
						}
					}
				})
			}
		}
	}
}

func checkGrokCanonicalUsage(t *testing.T, m Object, p string) {
	t.Helper()
	u := ReadUsage(m, p)
	if !u.Known || u.Input != 32 || u.Output != 103 || u.CacheRead != 4 || p != "anthropic" && (!u.ReasoningKnown || u.Reasoning != 94 || obj(m["usage"])["total_tokens"] != float64(135)) {
		t.Fatalf("invalid canonical %s usage: %+v (%+v)", p, u, m["usage"])
	}
}
