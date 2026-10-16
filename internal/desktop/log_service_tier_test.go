package desktop

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealLogBridgeRawServiceTier(t *testing.T) {
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","status":"completed","service_tier":"provider-turbo","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer up.Close()
	secret := "synthetic-bridge-key"
	if out := b.SaveProvider(SaveProvider{Provider: Provider{ID: "tiers", BaseURLs: []string{up.URL}, Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, Secret: &secret}); out.Problem != nil {
		t.Fatal(out)
	}
	if out := b.ProviderAction(Action{ProviderID: "tiers", ExpectedRevision: 1, Action: "test", Target: "default", Model: "demo", Approved: true}); out.Problem != nil || out.Data == nil || out.Data.Probe == nil || out.Data.Probe.Failure != "" {
		t.Fatal(out)
	}
	var id string
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		out := b.Logs(LogQuery{Source: "probe", Limit: 50})
		if out.Problem != nil || out.Data == nil {
			t.Fatal(out)
		}
		if len(out.Data.Requests) == 1 && out.Data.Requests[0].Status == "succeeded" {
			row := out.Data.Requests[0]
			if row.Evidence["upstream_service_tier_raw"] != `"provider-turbo"` || row.Evidence["sent_service_tier"] != "unspecified" || row.Evidence["fast_triggered"] != "unknown" {
				t.Fatalf("%+v", row)
			}
			if len(row.Attempts) != 0 || row.Evidence["body_bytes"] != "" {
				t.Fatal("list exposed full diagnostics")
			}
			id = row.ID
			break
		}
	}
	if id == "" {
		t.Fatal("log was not persisted")
	}
	detail := b.LogDetail(id)
	if detail.Problem != nil || detail.Data == nil || detail.Data.Evidence["upstream_service_tier_raw"] != `"provider-turbo"` {
		t.Fatal(detail)
	}
	if out := b.LogDetail("missing-request"); out.Problem == nil {
		t.Fatal("missing log did not fail")
	}
	path := filepath.Join(t.TempDir(), "tiers.jsonl")
	s.options.SaveLogFile = func() (string, error) { return path, nil }
	if out := b.ExportLogs(LogQuery{Source: "probe"}); out.Problem != nil || out.Data == nil || !out.Data.Accepted {
		t.Fatal(out)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), "provider-turbo") || strings.Contains(string(raw), secret) {
		t.Fatal("invalid or unsafe export", err)
	}
}
