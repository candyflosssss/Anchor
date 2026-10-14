package core

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
)

func probeCall(t *testing.T, f *fixture, in catalog.ProbeRequest) catalog.ProbeProgress {
	t.Helper()
	b, _ := json.Marshal(in)
	resp := request(t, "POST", f.control.URL+"/control/v1/providers/"+in.ProviderID+"/probe", f.owner, string(b))
	if resp.StatusCode != 200 {
		t.Fatalf("probe status %d: %s", resp.StatusCode, text(t, resp))
	}
	var out catalog.ProbeProgress
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return out
}
func awaitProbe(t *testing.T, f *fixture, in catalog.ProbeRequest, done func(catalog.ProbeProgress) bool) catalog.ProbeProgress {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		out := probeCall(t, f, in)
		if done(out) {
			return out
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("probe progress timed out")
	return catalog.ProbeProgress{}
}
func TestProviderProbeStreamsBeforeCompletion(t *testing.T) {
	f := setup(t)
	release := make(chan struct{})
	started := make(chan struct{})
	defer close(release)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["stream"] != true || r.Header.Get("Authorization") != "Bearer synthetic-probe-key" {
			t.Error("stream or key missing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"O\"}\n\n")
		w.(http.Flusher).Flush()
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"K\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n")
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic-probe-key")
	in := catalog.ProbeRequest{ProviderID: "p", ExpectedRevision: 1, Action: "start", KeyID: "default", Model: "demo", Approved: true}
	first := probeCall(t, f, in)
	in.Action = "read"
	in.RunID = first.RunID
	<-started
	partial := awaitProbe(t, f, in, func(p catalog.ProbeProgress) bool { return p.Text == "O" })
	if partial.Status != "running" || partial.Result != nil {
		t.Fatalf("not live: %+v", partial)
	}
	// Explicitly release before awaiting final status; retain a safe cleanup path.
	release <- struct{}{}
	final := awaitProbe(t, f, in, func(p catalog.ProbeProgress) bool { return p.Status != "running" })
	if final.Text != "OK" || final.Result == nil || final.Result.Failure != "" {
		t.Fatalf("final=%+v", final)
	}
	if err := f.core.logs.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	w, _ := f.core.workspace("p")
	if len(w.Usage) != 1 || w.Keys[0].Test == nil {
		t.Fatalf("probe did not share health/usage: %+v", w)
	}
}
func TestProviderProbeCancelAndGuards(t *testing.T) {
	f := setup(t)
	cancelled := make(chan struct{})
	entered := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic-key")
	in := catalog.ProbeRequest{ProviderID: "p", ExpectedRevision: 1, Action: "start", KeyID: "default", Model: "demo", Approved: true}
	for _, bad := range []catalog.ProbeRequest{{ProviderID: "p", ExpectedRevision: 2, Action: "start", KeyID: "default", Model: "demo", Approved: true}, {ProviderID: "p", ExpectedRevision: 1, Action: "start", KeyID: "missing", Model: "demo", Approved: true}, {ProviderID: "p", ExpectedRevision: 1, Action: "start", KeyID: "default", Model: "demo"}} {
		raw, _ := json.Marshal(bad)
		resp := request(t, "POST", f.control.URL+"/control/v1/providers/p/probe", f.owner, string(raw))
		if resp.StatusCode == 200 {
			t.Fatal("invalid start accepted")
		}
		resp.Body.Close()
	}
	run := probeCall(t, f, in)
	in.RunID = run.RunID
	in.Action = "cancel"
	<-entered
	probeCall(t, f, in)
	in.Action = "read"
	final := awaitProbe(t, f, in, func(p catalog.ProbeProgress) bool { return p.Status == "cancelled" })
	if final.Result == nil {
		t.Fatal("cancel missing result")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream was not cancelled")
	}
	in.ProviderID = "other"
	raw, _ := json.Marshal(in)
	resp := request(t, "POST", f.control.URL+"/control/v1/providers/other/probe", f.owner, string(raw))
	if resp.StatusCode == 200 {
		t.Fatal("cross-provider read accepted")
	}
	if strings.Contains(final.Text, "synthetic-key") {
		t.Fatal("secret leaked")
	}
}

func TestProbeProgressSharedContract(t *testing.T) {
	raw, err := os.ReadFile("../../desktop/contract/provider-probe.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Request     catalog.ProbeRequest  `json:"request"`
		Progress    catalog.ProbeProgress `json:"progress"`
		HTMLFailure catalog.ProbeProgress `json:"html_failure"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Request.ProviderID != fixture.Progress.ProviderID || fixture.Progress.Status != "running" || fixture.Progress.Result != nil {
		t.Fatal("fixture drift")
	}
	if fixture.HTMLFailure.Status != "completed" || fixture.HTMLFailure.Result == nil || fixture.HTMLFailure.Result.Failure != "unexpected_html" || fixture.HTMLFailure.Result.HTTPStatus != 200 {
		t.Fatal("HTML failure fixture drift")
	}
	encoded, _ := json.Marshal(fixture.Progress)
	var got, want any
	json.Unmarshal(encoded, &got)
	var value map[string]json.RawMessage
	json.Unmarshal(raw, &value)
	json.Unmarshal(value["progress"], &want)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("progress JSON drift")
	}
}
func TestProbeOutputRedactsSplitCredentials(t *testing.T) {
	var visible []string
	output := &probeOutput{header: http.Header{}, secret: "synthetic-key", notify: func(_, text string) { visible = append(visible, text) }}
	for _, delta := range []string{"hello syn", "thetic-", "key!"} {
		raw, _ := json.Marshal(map[string]string{"type": "response.output_text.delta", "delta": delta})
		if _, err := output.Write(append(append([]byte("data: "), raw...), '\n', '\n')); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(visible, "|") != "hello |hello |hello [redacted]!" {
		t.Fatal(visible)
	}
}

func TestProviderProbeStreamFailuresAndJSONFallback(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body, text, failure string
		status                                 int
	}{
		{"incomplete", "text/event-stream", "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\"}}\n\n", "", "invalid_response", 200},
		{"no-terminal", "text/event-stream", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n", "partial", "invalid_response", 200},
		{"auth", "application/json", "{\"error\":\"private-error-synthetic-key\"}", "", "authentication", 401},
		{"json-reasoning", "application/json", `{"id":"r","status":"completed","output":[{"type":"reasoning","summary":[]},{"type":"message","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":3,"output_tokens":2}}`, "OK", "", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer up.Close()
			f.add(t, "p", up.URL, "synthetic-key")
			in := catalog.ProbeRequest{ProviderID: "p", ExpectedRevision: 1, Action: "start", KeyID: "default", Model: "demo", Approved: true}
			initial := probeCall(t, f, in)
			in.Action = "read"
			in.RunID = initial.RunID
			final := awaitProbe(t, f, in, func(p catalog.ProbeProgress) bool { return p.Status == "completed" })
			if final.Text != tc.text || final.Result.Failure != tc.failure {
				t.Fatalf("final=%+v", final)
			}
		})
	}
}
