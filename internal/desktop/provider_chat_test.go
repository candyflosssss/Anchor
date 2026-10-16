package desktop

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestProviderChatBridge(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []ChatMessage `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Input) != 3 {
			t.Error("bridge lost history")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	defer up.Close()
	secret := "synthetic-chat-secret"
	saved := b.SaveProvider(SaveProvider{Provider: Provider{ID: "chat-demo", BaseURLs: []string{up.URL}, Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, Secret: &secret})
	if saved.Problem != nil {
		t.Fatal(saved)
	}
	raw, _ := os.ReadFile("../../desktop/contract/chat.json")
	var fixture struct {
		Request ChatRequest `json:"request"`
	}
	if json.Unmarshal(raw, &fixture) != nil {
		t.Fatal("fixture")
	}
	input := fixture.Request
	invalid := input
	invalid.Approved = false
	if r := b.ProviderChat(invalid); r.Problem == nil || r.Problem.Code != "invalid" {
		t.Fatal("unapproved accepted")
	}
	first := b.ProviderChat(input)
	if first.Problem != nil || first.Data == nil {
		t.Fatal(first)
	}
	input.Action = "read"
	input.RunID = first.Data.RunID
	input.Messages = nil
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		next := b.ProviderChat(input)
		if next.Problem != nil || next.Data == nil {
			t.Fatal(next)
		}
		if next.Data.Status == "completed" {
			if next.Data.Text != "OK" || next.Data.Result.Failure != "" {
				t.Fatal(next)
			}
			return
		}
	}
	t.Fatal("chat bridge timed out")
}
