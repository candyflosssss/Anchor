package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
)

func chatInput(t *testing.T) catalog.ChatRequest {
	t.Helper()
	raw, err := os.ReadFile("../../desktop/contract/chat.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Request catalog.ChatRequest `json:"request"`
	}
	if json.Unmarshal(raw, &f) != nil {
		t.Fatal("invalid fixture")
	}
	f.Request.ProviderID = "p"
	return f.Request
}
func chatCall(t *testing.T, f *fixture, in catalog.ChatRequest) catalog.ProbeProgress {
	t.Helper()
	raw, _ := json.Marshal(in)
	resp := request(t, "POST", f.control.URL+"/control/v1/providers/"+in.ProviderID+"/chat", f.owner, string(raw))
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("chat status %d: %s", resp.StatusCode, text(t, resp))
	}
	var out catalog.ProbeProgress
	if json.NewDecoder(resp.Body).Decode(&out) != nil {
		t.Fatal("invalid chat response")
	}
	return out
}
func awaitChat(t *testing.T, f *fixture, in catalog.ChatRequest, done func(catalog.ProbeProgress) bool) catalog.ProbeProgress {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		out := chatCall(t, f, in)
		if done(out) {
			return out
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("chat timed out")
	return catalog.ProbeProgress{}
}
func TestProviderChatProtocolsHistoryAndPrivacy(t *testing.T) {
	for _, protocol := range []string{"responses", "chat-completions", "anthropic"} {
		t.Run(protocol, func(t *testing.T) {
			f := setup(t)
			in := chatInput(t)
			messages := append([]catalog.ChatMessage(nil), in.Messages...)
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				if body["stream"] != true || !strings.Contains(string(raw), messages[0].Content) || !strings.Contains(string(raw), messages[2].Content) {
					t.Errorf("missing history or streaming: %s", raw)
				}
				if protocol == "anthropic" {
					if r.Header.Get("x-api-key") != "synthetic-chat-secret" {
						t.Error("wrong key")
					}
				} else if r.Header.Get("Authorization") != "Bearer synthetic-chat-secret" {
					t.Error("wrong key")
				}
				w.Header().Set("Content-Type", "application/json")
				switch protocol {
				case "responses":
					io.WriteString(w, `{"id":"r","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"reply synthetic-chat-secret"}]}],"usage":{"input_tokens":4,"output_tokens":2}}`)
				case "chat-completions":
					io.WriteString(w, `{"id":"r","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"reply synthetic-chat-secret"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`)
				case "anthropic":
					io.WriteString(w, `{"id":"r","type":"message","role":"assistant","content":[{"type":"text","text":"reply synthetic-chat-secret"}],"stop_reason":"end_turn","usage":{"input_tokens":4,"output_tokens":2}}`)
				}
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "synthetic-chat-secret")
			if protocol != "responses" {
				if err := f.store.MutateProvider("p", 1, "key-put", "default", "", "", true, false, protocol); err != nil {
					t.Fatal(err)
				}
				in.ExpectedRevision = 2
			}
			first := chatCall(t, f, in)
			in.Action = "read"
			in.RunID = first.RunID
			in.Messages = nil
			out := awaitChat(t, f, in, func(p catalog.ProbeProgress) bool { return p.Status != "running" })
			if out.Result == nil || out.Result.Failure != "" || out.Text != "reply [redacted]" {
				t.Fatalf("final: %+v", out)
			}
			if err := f.core.logs.Flush(t.Context()); err != nil {
				t.Fatal(err)
			}
			// Check every task-created state/log file for prompt and generated content.
			for _, forbidden := range []string{"请记住我们在测试对话", "reply synthetic-chat-secret", "reply [redacted]"} {
				if strings.Contains(f.logs.String(), forbidden) {
					t.Fatal("chat content leaked to system logs")
				}
				_ = filepath.WalkDir(f.store.Dir, func(path string, entry os.DirEntry, err error) error {
					if err == nil && !entry.IsDir() {
						data, err := os.ReadFile(path)
						if err != nil {
							t.Fatal(err)
						}
						if strings.Contains(string(data), forbidden) {
							t.Fatalf("chat content leaked to %s", entry.Name())
						}
					}
					return err
				})
			}
		})
	}
}
func TestProviderChatStreamCancelAfterRevisionChange(t *testing.T) {
	f := setup(t)
	in := chatInput(t)
	started := make(chan struct{})
	ended := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(ended)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic-chat-secret")
	first := chatCall(t, f, in)
	in.Action = "read"
	in.RunID = first.RunID
	in.Messages = nil
	<-started
	out := awaitChat(t, f, in, func(p catalog.ProbeProgress) bool { return p.Text == "partial" })
	if out.Status != "running" {
		t.Fatal("not streaming")
	}
	if err := f.store.MutateProvider("p", 1, "key-put", "other", "synthetic-other", "", true, false); err != nil {
		t.Fatal(err)
	}
	in.Action = "cancel"
	chatCall(t, f, in)
	in.Action = "read"
	out = awaitChat(t, f, in, func(p catalog.ProbeProgress) bool { return p.Status == "cancelled" })
	if out.Text != "partial" {
		t.Fatal("partial reply lost")
	}
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("upstream not cancelled")
	}
}
func TestProviderChatRejectsBeforeUpstreamAndNeverFallsBack(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer synthetic-chosen" {
			t.Error("wrong selected Key")
		}
		w.WriteHeader(401)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic-first")
	if err := f.store.MutateProvider("p", 1, "key-put", "chosen", "synthetic-chosen", "", true, false); err != nil {
		t.Fatal(err)
	}
	input := chatInput(t)
	input.ExpectedRevision = 2
	for _, tc := range []struct {
		change func(*catalog.ChatRequest)
		code   int
	}{
		{func(in *catalog.ChatRequest) { in.Approved = false }, 400},
		{func(in *catalog.ChatRequest) { in.ExpectedRevision = 1 }, 409},
		{func(in *catalog.ChatRequest) { in.KeyID = "missing" }, 400},
		{func(in *catalog.ChatRequest) { in.Messages = nil }, 400},
	} {
		in := input
		tc.change(&in)
		raw, _ := json.Marshal(in)
		resp := request(t, "POST", f.control.URL+"/control/v1/providers/p/chat", f.owner, string(raw))
		if resp.StatusCode != tc.code {
			t.Fatalf("status %d want %d", resp.StatusCode, tc.code)
		}
		resp.Body.Close()
	}
	raw, _ := json.Marshal(input)
	resp := request(t, "POST", f.control.URL+"/control/v1/providers/p/chat", f.token, string(raw))
	if resp.StatusCode != 401 {
		t.Fatalf("gateway token got %d", resp.StatusCode)
	}
	resp.Body.Close()
	if calls.Load() != 0 {
		t.Fatal("invalid requests reached upstream")
	}
	input.KeyID = "chosen"
	first := chatCall(t, f, input)
	input.RunID = first.RunID
	input.Action = "read"
	input.Messages = nil
	final := awaitChat(t, f, input, func(p catalog.ProbeProgress) bool { return p.Status != "running" })
	if final.Result.Failure != "authentication" || calls.Load() != 1 {
		t.Fatal("inference retried or fallback key used")
	}
}

func TestProviderChatOutputBoundsAndSplitSecret(t *testing.T) {
	for _, size := range []int{17000, catalog.ChatMaxOutputBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			f := setup(t)
			input := chatInput(t)
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for _, delta := range []string{"synthetic-", "chat-secret ", strings.Repeat("x", size)} {
					raw, _ := json.Marshal(map[string]string{"type": "response.output_text.delta", "delta": delta})
					fmt.Fprintf(w, "data: %s\n\n", raw)
				}
				io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "synthetic-chat-secret")
			first := chatCall(t, f, input)
			input.Action = "read"
			input.RunID = first.RunID
			input.Messages = nil
			final := awaitChat(t, f, input, func(p catalog.ProbeProgress) bool { return p.Status != "running" })
			if strings.Contains(final.Text, "synthetic-chat-secret") {
				t.Fatal("split secret leaked")
			}
			if size == 17000 {
				if final.Result.Failure != "" || final.Text != "[redacted] "+strings.Repeat("x", size) {
					t.Fatal("chat retained old 16 KiB probe cap")
				}
			} else if final.Result.Failure != "invalid_response" {
				t.Fatal("oversized chat accepted")
			}
		})
	}
}

func TestChatRedactionDoesNotExpandBeyondPublicLimit(t *testing.T) {
	output := &probeOutput{secret: "x", limit: 64, strict: true, notify: func(_, text string) {
		if len(text) > 64 {
			t.Fatal("oversized redacted output published")
		}
	}}
	raw := []byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"xxxxxxxxxx\"}\n\n")
	if _, err := output.Write(raw); err == nil {
		t.Fatal("redaction expansion exceeded public output limit")
	}
}
