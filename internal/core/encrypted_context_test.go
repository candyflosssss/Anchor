package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/state"
)

const encryptedContextRejection = `{"error":{"code":"invalid_encrypted_content","message":"The encrypted content for item rs_private could not be verified. Reason: Encrypted content could not be decrypted or parsed."}}`
const encryptedContextFailedEvent = `{"type":"response.failed","response":{"error":{"message":"The encrypted content for item rs_private could not be verified. Reason: Encrypted content could not be decrypted or parsed."}}}`
const encryptedContextInput = `[{"role":"user","content":"private prompt"},{"type":"reasoning","id":"rs_private","encrypted_content":"private ciphertext"},{"type":"reasoning","summary":[]},{"type":"function_call","call_id":"call_1","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"private tool result"},{"type":"compaction","encrypted_content":"private compact"},{"type":"agent_message","content":[{"type":"encrypted_content","encrypted_content":"private task"}]},{"role":"user","content":"continue"}]`

func TestEncryptedContextErrorRecognition(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{encryptedContextRejection, "invalid_encrypted_content"},
		{encryptedContextFailedEvent, "encrypted_content_unverifiable"},
		{`{"type":"error","message":"stream disconnected before completion: The encrypted content for item rs_private could not be verified. Reason: Encrypted content could not be decrypted or parsed."}`, "encrypted_content_unverifiable"},
		{`{"type":"response.failed","response":{"error":{"code":"invalid_encrypted_content"},"output":[{"type":"reasoning","summary":[]}]}}`, "invalid_encrypted_content"},
		{`{"error":{"code":"invalid_encrypted_content"},"output":[{"type":"reasoning","encrypted_content":"new output"}]}`, ""},
		{`{"error":{"code":"invalid_encrypted_content"},"output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}`, ""},
		{`{"type":"error","message":"reasoning encrypted_content was not issued to this caller"}`, "encrypted_content_caller_mismatch"},
		{`{"error":{"message":"The encrypted content gAAA could not be verified."}}`, "encrypted_content_unverifiable"},
		{`{"message":"The encrypted content could not be verified."}`, ""},
		{`{"output":[{"text":"invalid_encrypted_content"}]}`, ""},
		{`{"error":{"code":"invalid_encrypted_content"},"output":[{"type":"function_call"}]}`, ""},
		{`{"error":{"message":"connection reset while decoding"}}`, ""},
		{`{"error":{"message":"encrypted_content is required"}}`, ""},
		{`{"error":{"code":"invalid_api_key"}}`, ""},
		{`{"error":"invalid_encrypted_content"}`, ""},
		{`<html>invalid_encrypted_content</html>`, ""},
	} {
		if got := encryptedContextError([]byte(tc.body)); got != tc.want {
			t.Errorf("recognition got %q want %q for %s", got, tc.want, tc.body)
		}
	}
}

func TestEncryptedContextCleanupPreservesConversationAndTasks(t *testing.T) {
	var request map[string]json.RawMessage
	json.Unmarshal([]byte(`{"model":"m","input":`+encryptedContextInput+`,"instructions":"private instructions"}`), &request)
	original, _ := json.Marshal(request)
	if !hasEncryptedReasoning(request) {
		t.Fatal("encrypted reasoning not detected")
	}
	recovery := recoverEncryptedContext(request, "invalid_encrypted_content")
	if recovery == nil || recovery.removed != 2 {
		t.Fatal(recovery)
	}
	var before, after []json.RawMessage
	json.Unmarshal(request["input"], &before)
	json.Unmarshal(recovery.request["input"], &after)
	want := append(append([]json.RawMessage{}, before[:1]...), before[3:]...)
	if !reflect.DeepEqual(after, want) || string(request["instructions"]) != string(recovery.request["instructions"]) {
		t.Fatalf("messages, tool pairs, compaction or task changed: %s", recovery.request["input"])
	}
	now, _ := json.Marshal(request)
	if !bytes.Equal(now, original) || hasEncryptedReasoning(recovery.request) {
		t.Fatal("original request mutated or cleanup incomplete")
	}
	if recoverEncryptedContext(recovery.request, recovery.reason) != nil || recoverEncryptedContext(request, "") != nil {
		t.Fatal("cleanup without rejection or removable items")
	}
	for _, input := range []string{`"hi"`, `[]`, `[{"type":"compaction","encrypted_content":"cipher"}]`, `[{"type":"reasoning"}]`} {
		if hasEncryptedReasoning(map[string]json.RawMessage{"input": json.RawMessage(input)}) {
			t.Fatal("unexpected eligible input", input)
		}
	}
}

func TestEncryptedContextStreamPriming(t *testing.T) {
	lifecycle := ": heartbeat\r\n\r\nevent: response.created\r\ndata: {\"type\":\"response.created\",\"response\":{\"output\":[]}}\r\n\r\n"
	failure := "data: " + encryptedContextFailedEvent + "\n\n"
	for _, tc := range []struct{ name, wire, want string }{
		{"early rejection", lifecycle + failure, "encrypted_content_unverifiable"},
		{"codex metadata before rejection", `data: {"type":"codex.response.metadata","metadata":{"turn_id":"private-metadata"}}` + "\n\n" + lifecycle + failure, "encrypted_content_unverifiable"},
		{"event-only codex metadata", "event: codex.response.metadata\ndata: {\"metadata\":{}}\n\n" + failure, "encrypted_content_unverifiable"},
		{"unknown metadata remains boundary", `data: {"type":"private.response.metadata","metadata":{}}` + "\n\n" + failure, ""},
		{"empty reasoning shell", lifecycle + `data: {"type":"response.output_item.added","item":{"type":"reasoning","id":"rs_private","status":"in_progress","summary":[],"encrypted_content":null}}` + "\n\n" + failure, "encrypted_content_unverifiable"},
		{"empty message shell", `data: {"type":"response.output_item.added","item":{"type":"message","role":"assistant","content":[]}}` + "\n\n" + failure, "encrypted_content_unverifiable"},
		{"empty summary part", `data: {"type":"response.reasoning_summary_part.added","part":{"type":"summary_text","text":""}}` + "\n\n" + failure, "encrypted_content_unverifiable"},
		{"empty text part", `data: {"type":"response.content_part.added","part":{"type":"output_text","text":"","annotations":[],"logprobs":[]}}` + "\n\n" + failure, "encrypted_content_unverifiable"},
		{"event-only lifecycle", "event: response.created\ndata: {\"response\":{\"output\":[]}}\n\n" + failure, "encrypted_content_unverifiable"},
		{"event-only failure", "event: response.failed\ndata: {\"response\":{\"error\":{\"code\":\"invalid_encrypted_content\"}}}\n\n", "invalid_encrypted_content"},
		{"reasoning item has content", `data: {"type":"response.output_item.added","item":{"type":"reasoning","summary":[{"text":"thinking"}]}}` + "\n\n" + failure, ""},
		{"reasoning item has ciphertext", `data: {"type":"response.output_item.added","item":{"type":"reasoning","encrypted_content":"new output"}}` + "\n\n" + failure, ""},
		{"unknown item field", `data: {"type":"response.output_item.added","item":{"type":"reasoning","private_payload":"generated"}}` + "\n\n" + failure, ""},
		{"summary part has content", `data: {"type":"response.reasoning_summary_part.added","part":{"type":"summary_text","text":"thinking"}}` + "\n\n" + failure, ""},
		{"event-only type", "event: error\ndata: {\"code\":\"invalid_encrypted_content\"}\n\n", "invalid_encrypted_content"},
		{"multiline", "data: {\"type\":\"error\",\ndata: \"code\":\"invalid_encrypted_content\"}\n\n", "invalid_encrypted_content"},
		{"text already started", lifecycle + "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" + failure, ""},
		{"tool shell already started", "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\"}}\n\n" + failure, ""},
		{"reasoning already started", "data: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"thinking\"}\n\n" + failure, ""},
		{"lifecycle has output", "data: {\"type\":\"response.created\",\"response\":{\"output\":[{\"type\":\"function_call\"}]}}\n\n" + failure, ""},
		{"unrelated error", "data: {\"type\":\"error\",\"message\":\"network unavailable\"}\n\n", ""},
		{"malformed", "data: invalid\n\n" + failure, ""},
		{"incomplete event", "data: " + encryptedContextFailedEvent, ""},
		{"EOF", lifecycle, ""},
		{"buffer cap", strings.Repeat(":", encryptedContextPrefixLimit) + "\n\n" + failure, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Tiny reads exercise frame and CRLF boundaries.
			var source io.Reader = &smallEncryptedContextReader{reader: strings.NewReader(tc.wire)}
			if tc.name == "buffer cap" {
				source = strings.NewReader(tc.wire)
			}
			reader, reason, inspection, event := primeEncryptedContextStream(t.Context(), source)
			if reason != tc.want || inspection == "" || event == "" {
				t.Fatalf("got reason %q want %q", reason, tc.want)
			}
			got, err := io.ReadAll(reader)
			if err != nil || string(got) != tc.wire {
				t.Fatalf("inspected stream was not preserved: %v", err)
			}
		})
	}
}

type smallEncryptedContextReader struct{ reader io.Reader }

func (s *smallEncryptedContextReader) Read(p []byte) (int, error) {
	return s.reader.Read(p[:min(len(p), 13)])
}

func TestEncryptedContextRequestDeadlineHandsOffPendingRead(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ready := make(chan struct{})
	go func() {
		io.WriteString(writer, "data: {\"type\":\"response.created\"}\n\n")
		close(ready)
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	stream, reason, inspection, _ := primeEncryptedContextStream(ctx, reader)
	<-ready
	if reason != "" || inspection != "timeout" {
		t.Fatal(reason, inspection)
	}
	go func() {
		io.WriteString(writer, "data: "+encryptedContextFailedEvent+"\n\n")
		writer.Close()
	}()
	got, err := io.ReadAll(stream)
	if err != nil || !strings.Contains(string(got), encryptedContextFailedEvent) {
		t.Fatal("pending read lost", string(got), err)
	}
}

func TestEncryptedContextPrimingCancellationUnblocks(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	stream, reason, inspection, _ := primeEncryptedContextStream(ctx, reader)
	writer.CloseWithError(context.Canceled)
	_, err := io.ReadAll(stream)
	if reason != "" || inspection != "cancelled" || !errors.Is(err, context.Canceled) {
		t.Fatal(reason, err)
	}
}

func TestGatewayEncryptedContextRecoveryAndLogs(t *testing.T) {
	for _, mode := range []string{"http400", "http422", "json200", "sse200"} {
		for _, retryFails := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fail=%t", mode, retryFails), func(t *testing.T) {
				f := setup(t)
				var calls atomic.Int32
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := calls.Add(1)
					var body map[string]json.RawMessage
					json.NewDecoder(r.Body).Decode(&body)
					if (n == 1) != hasEncryptedReasoning(body) {
						t.Error("first request must preserve, second must remove encrypted reasoning")
					}
					if r.Header.Get("Authorization") != "Bearer synthetic-upstream-credential" || string(body["model"]) != `"up"` {
						t.Error("recovery changed credential/model")
					}
					if n == 1 || retryFails {
						switch mode {
						case "http400", "http422":
							w.Header().Set("Content-Type", "application/json")
							status := 400
							if mode == "http422" {
								status = 422
							}
							w.WriteHeader(status)
							io.WriteString(w, encryptedContextRejection)
						case "json200":
							w.Header().Set("Content-Type", "application/json")
							io.WriteString(w, encryptedContextRejection)
						case "sse200":
							w.Header().Set("Content-Type", "text/event-stream")
							io.WriteString(w, "data: {\"type\":\"response.created\"}\n\ndata: "+encryptedContextFailedEvent+"\n\n")
						}
						return
					}
					if mode == "sse200" {
						w.Header().Set("Content-Type", "text/event-stream")
						io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						io.WriteString(w, `{"id":"r","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":1}}`)
					}
				}))
				defer up.Close()
				f.add(t, "p", up.URL, "synthetic-upstream-credential")
				f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
				resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, fmt.Sprintf(`{"model":"public","stream":%t,"input":%s}`, mode == "sse200", encryptedContextInput))
				body := text(t, resp)
				if calls.Load() != 2 || (!retryFails && (resp.StatusCode != 200 || strings.Contains(body, "could not be verified") || strings.Contains(body, "response.created"))) {
					// The discarded first stream's lifecycle/errors must not leak downstream.
					t.Fatal(calls.Load(), resp.StatusCode, body)
				}
				if err := f.core.logs.Flush(t.Context()); err != nil {
					t.Fatal(err)
				}
				c := logClient(t, f)
				row, err := c.LogDetail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
				want := "succeeded"
				if retryFails {
					want = "failed"
				}
				if err != nil || row.Status != want || row.Evidence["encrypted_context_recovery"] != want || row.Evidence["encrypted_context_removed_reasoning"] != "2" || row.Evidence["encrypted_context_retry_count"] != "1" || row.AttemptCount != 2 || len(row.Attempts) != 2 || row.Attempts[0].Code != "encrypted_context_rejected" || row.Attempts[0].Outcome != "retry" || row.Attempts[1].Outcome != want {
					t.Fatalf("recovery diagnostics: %+v %v", row, err)
				}
				if row.Input != nil || row.Output != nil || row.Attempts[1].Input == nil && !retryFails {
					t.Fatal("unknown first-attempt usage must not become a known total", row)
				}
				if retryFails {
					if row.KnownUsage != nil {
						t.Fatal("failed attempts without usage must not create a subtotal", row.KnownUsage)
					}
				} else if row.KnownUsage == nil || *row.KnownUsage != (logbook.KnownUsage{Input: 3, Output: 1, Attempts: 1}) {
					t.Fatalf("confirmed recovery usage missing: %+v", row.KnownUsage)
				}
				page, err := c.Logs(t.Context(), logbook.Query{})
				if err != nil || len(page.Requests) != 1 || page.Requests[0].Evidence["encrypted_context_recovery"] != want || len(page.Requests[0].Attempts) != 0 {
					t.Fatal("list recovery marker missing", page, err)
				}
				if !retryFails && (page.Requests[0].KnownUsage == nil || *page.Requests[0].KnownUsage != *row.KnownUsage) {
					t.Fatal("compact list lost confirmed recovery usage", page.Requests[0])
				}
				var exported bytes.Buffer
				if err := c.ExportLogs(t.Context(), logbook.Query{}, &exported); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(exported.String(), `"encrypted_context_recovery":"`+want+`"`) {
					t.Fatal("export marker missing")
				}
				for _, forbidden := range []string{"private", "ciphertext", "could not be verified", "synthetic-upstream-credential", "rs_private"} {
					if strings.Contains(exported.String(), forbidden) || strings.Contains(f.logs.String(), forbidden) {
						t.Fatal("unsafe recovery logging", forbidden)
					}
				}
			})
		}
	}
}

func TestGatewayEncryptedContextNoUnsafeReplay(t *testing.T) {
	for _, tc := range []struct {
		name, prefix, errorBody string
		status                  int
		stream, noReasoning     bool
	}{
		{name: "unrelated error", status: 400, errorBody: `{"error":{"message":"bad tool schema"}}`},
		{name: "authentication", status: 401, errorBody: encryptedContextRejection},
		{name: "rate limit", status: 429, errorBody: encryptedContextRejection},
		{name: "server error", status: 500, errorBody: encryptedContextRejection},
		{name: "no reasoning", status: 400, errorBody: encryptedContextRejection, noReasoning: true},
		{name: "output already sent", status: 200, stream: true, prefix: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n", errorBody: "data: " + encryptedContextFailedEvent + "\n\n"},
		{name: "tool already sent", status: 200, stream: true, prefix: "data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\"}}\n\n", errorBody: "data: " + encryptedContextFailedEvent + "\n\n"},
		{name: "incomplete stream", status: 200, stream: true, errorBody: "data: {\"type\":\"response.created\"}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				contentType := "application/json"
				if tc.stream {
					contentType = "text/event-stream"
				}
				w.Header().Set("Content-Type", contentType)
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.prefix+tc.errorBody)
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "synthetic")
			f.model(t, state.Candidate{ProviderID: "p", Model: "m"})
			input := encryptedContextInput
			if tc.noReasoning {
				input = `[{"role":"user","content":"hi"}]`
			}
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, fmt.Sprintf(`{"model":"public","input":%s,"stream":%t}`, input, tc.stream))
			body := text(t, resp)
			if calls.Load() != 1 || tc.prefix != "" && !strings.HasPrefix(body, tc.prefix) {
				t.Fatal("unsafe replay or lost stream bytes", calls.Load(), body)
			}
			f.core.logs.Flush(t.Context())
			row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
			if err != nil || row.AttemptCount != 1 || row.Evidence["encrypted_context_recovery"] != "" {
				t.Fatal(row, err)
			}
			if tc.stream {
				want := "non_lifecycle_event"
				if tc.name == "incomplete stream" {
					want = "eof"
				}
				if row.Evidence["encrypted_context_stream_inspection"] != want {
					t.Fatal("missing safe inspection reason", row.Evidence)
				}
			}
		})
	}
}

func TestGatewayEncryptedContextRecoveryKeepsEndpointAndBudget(t *testing.T) {
	for _, expires := range []bool{false, true} {
		t.Run(fmt.Sprint(expires), func(t *testing.T) {
			f := setup(t)
			var calls, otherCalls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if n == 1 {
					if expires {
						time.Sleep(600 * time.Millisecond)
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(400)
					io.WriteString(w, encryptedContextRejection)
					return
				}
				if expires {
					select {
					case <-r.Context().Done():
						return
					case <-time.After(time.Second):
					}
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"id":"r","output":[]}`)
			}))
			defer up.Close()
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				otherCalls.Add(1)
				w.WriteHeader(500)
			}))
			defer other.Close()
			f.add(t, "p", up.URL, "synthetic")
			provider, _ := f.store.Provider("p")
			provider.BaseURLs = []string{up.URL, other.URL}
			if expires {
				provider.TimeoutSeconds = 1
			}
			if _, err := f.store.PutProvider(provider, provider.Revision, nil); err != nil {
				t.Fatal(err)
			}
			f.model(t, state.Candidate{ProviderID: "p", Model: "m"})
			resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":`+encryptedContextInput+`}`)
			text(t, resp)
			want := 200
			if expires {
				want = 504
			}
			if calls.Load() != 2 || otherCalls.Load() != 0 || resp.StatusCode != want {
				t.Fatal("recovery changed endpoint or reset timeout budget", calls.Load(), otherCalls.Load(), resp.StatusCode)
			}
		})
	}
}

func TestGatewayEncryptedContextRecoveryOnceAcrossFallback(t *testing.T) {
	f := setup(t)
	var first, second atomic.Int32
	f.add(t, "first", "https://first.invalid/v1", "synthetic")
	f.add(t, "second", "https://second.invalid/v1", "synthetic")
	f.model(t, state.Candidate{ProviderID: "first", Model: "up"}, state.Candidate{ProviderID: "second", Model: "up"})
	f.core.upstream.Transport = channelRetryTestTransport(func(r *http.Request) (*http.Response, error) {
		status, body := 400, encryptedContextRejection
		if r.URL.Host == "first.invalid" {
			if first.Add(1) == 2 {
				status, body = 503, ""
			}
		} else {
			second.Add(1)
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":`+encryptedContextInput+`}`)
	text(t, resp)
	f.core.logs.Flush(t.Context())
	row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
	if err != nil || first.Load() != 2 || second.Load() != 1 || row.AttemptCount != 3 || row.Evidence["encrypted_context_retry_count"] != "1" || row.Evidence["encrypted_context_recovery"] != "failed" {
		t.Fatal("cleanup retry exceeded the root request budget", first.Load(), second.Load(), row, err)
	}
}

func TestGatewayEncryptedContextRecoveryCancellationIsLogged(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	seen := make(chan struct{})
	f.add(t, "p", "https://upstream.invalid/v1", "synthetic")
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	f.core.upstream.Transport = channelRetryTestTransport(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(encryptedContextRejection))}, nil
		}
		close(seen)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := httptest.NewRequest("POST", f.gateway.URL+"/v1/responses", strings.NewReader(`{"model":"public","input":`+encryptedContextInput+`}`)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		f.core.Gateway().ServeHTTP(w, r)
		close(done)
	}()
	select {
	case <-seen:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery attempt did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery ignored cancellation")
	}
	f.core.logs.Flush(t.Context())
	row, err := f.core.logs.Detail(t.Context(), w.Header().Get("X-Anchor-Request-ID"))
	if err != nil || calls.Load() != 2 || row.Status != "cancelled" || row.Evidence["encrypted_context_recovery"] != "cancelled" || row.AttemptCount != 2 {
		t.Fatal("cancelled recovery marker missing", row, err)
	}
}

// The upstream may flush HTTP 200/lifecycle events while it validates history.
// A definite rejection arriving after two seconds is still before any output.
func TestGatewayEncryptedContextRecoveryAfterDelayedValidation(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		json.NewDecoder(r.Body).Decode(&body)
		n := calls.Add(1)
		if (n == 1) != hasEncryptedReasoning(body) {
			t.Error("only the recovery request should remove reasoning")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if n == 1 {
			io.WriteString(w, `data: {"type":"codex.response.metadata","metadata":{"turn_id":"private-metadata"}}`+"\n\n")
			w.(http.Flusher).Flush()
			io.WriteString(w, ": heartbeat\n\n")
			// Real lifecycle frames may echo long instructions and tool schemas.
			fmt.Fprintf(w, "data: %s\n\n", fmt.Sprintf(`{"type":"response.created","response":{"output":[],"instructions":%q}}`, strings.Repeat("synthetic metadata ", 5000)))
			io.WriteString(w, `data: {"type":"response.output_item.added","item":{"type":"reasoning","id":"rs_private","status":"in_progress","summary":[]}}`+"\n\n")
			io.WriteString(w, `data: {"type":"response.reasoning_summary_part.added","part":{"type":"summary_text","text":""}}`+"\n\n")
			w.(http.Flusher).Flush()
			select {
			case <-time.After(2200 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
			io.WriteString(w, `event: response.failed`+"\n"+`data: {"response":{"error":{"code":"invalid_encrypted_content"},"output":[{"type":"reasoning","summary":[]}]}}`+"\n\n")
			return
		}
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"output\":[]}}\n\n")
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","stream":true,"input":`+encryptedContextInput+`}`)
	body := text(t, resp)
	if resp.StatusCode != 200 || calls.Load() != 2 || !strings.Contains(body, "OK") || strings.Contains(body, "could not be verified") || strings.Contains(body, "private-metadata") {
		t.Fatal("late validation bypassed cleanup", calls.Load(), resp.StatusCode, body)
	}
	f.core.logs.Flush(t.Context())
	row, err := f.core.logs.Detail(t.Context(), resp.Header.Get("X-Anchor-Request-ID"))
	if err != nil || row.Status != "succeeded" || row.Evidence["encrypted_context_recovery"] != "succeeded" || row.Evidence["encrypted_context_stream_inspection"] != "rejected" || row.AttemptCount != 2 || row.Evidence["encrypted_context_stream_event"] != "error" {
		t.Fatal("late recovery marker missing", row, err)
	}
	page, err := logClient(t, f).Logs(t.Context(), logbook.Query{})
	if err != nil || len(page.Requests) != 1 || page.Requests[0].Evidence["encrypted_context_stream_inspection"] != "rejected" || page.Requests[0].Evidence["encrypted_context_stream_event"] != "error" {
		t.Fatal("list inspection marker missing", page, err)
	}
}
