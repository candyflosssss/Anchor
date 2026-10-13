package desktop

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProviderProbeBridge(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	defer up.Close()
	secret := "synthetic-bridge-key"
	saved := b.SaveProvider(SaveProvider{Provider: Provider{ID: "probe", BaseURLs: []string{up.URL}, Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, Secret: &secret})
	if saved.Problem != nil {
		t.Fatal(saved)
	}
	input := ProbeRequest{ProviderID: "probe", ExpectedRevision: 1, Action: "start", KeyID: "default", Model: "demo", Approved: true}
	first := b.ProviderProbe(input)
	if first.Problem != nil || first.Data == nil || first.Data.RunID == "" {
		t.Fatal(first)
	}
	input.Action = "read"
	input.RunID = first.Data.RunID
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		result := b.ProviderProbe(input)
		if result.Problem != nil || result.Data == nil {
			t.Fatal(result)
		}
		if result.Data.Status == "completed" {
			if result.Data.Text != "OK" || result.Data.Result.Failure != "" {
				t.Fatal(result)
			}
			return
		}
	}
	t.Fatal("bridge probe timed out")
}
