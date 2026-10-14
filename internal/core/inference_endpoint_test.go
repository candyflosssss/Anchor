package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/logbook"
	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestInferenceEndpointResolution(t *testing.T) {
	for _, tc := range []struct {
		name, base, protocol, media, body string
		status, calls                     int
	}{
		{"responses", "", "responses", "text/html", "<html>private-page</html>", 200, 2},
		{"chat-prefix", "/proxy", "chat-completions", "text/html", "<html>private-page</html>", 200, 2},
		{"anthropic", "", "anthropic", "application/xhtml+xml", "<html>private-page</html>", 200, 2},
		{"html-404", "", "responses", "text/html", "<html>private-page</html>", 404, 2},
		{"html-405", "", "responses", "text/html", "<html>private-page</html>", 405, 2},
		{"mislabelled-html", "", "responses", "application/json", "\ufeff \n<!DOCTYPE HTML><html>private-page</html>", 200, 2},
		{"versioned", "/v1", "responses", "text/html", "<html>private-page</html>", 200, 1},
		{"other-version", "/prefix/v2beta1/openai", "responses", "text/html", "<html>private-page</html>", 200, 1},
		{"working-root", "", "responses", "application/json", `{"id":"valid"}`, 200, 1},
		{"json-labelled-html", "", "responses", "text/html", `{"error":"bad"}`, 200, 1},
		{"invalid-json", "", "responses", "application/json", `bad-json`, 200, 1},
		{"json-error", "", "responses", "application/json", `{"error":"bad"}`, 404, 1},
		{"auth", "", "responses", "text/html", "<html>private-page</html>", 401, 1},
		{"forbidden", "", "responses", "text/html", "<html>private-page</html>", 403, 1},
		{"rate-limit", "", "responses", "text/html", "<html>private-page</html>", 429, 1},
		{"server-error", "", "responses", "text/html", "<html>private-page</html>", 500, 1},
		{"redirect", "", "responses", "text/html", "<html>private-page</html>", 302, 1},
		{"sse", "", "responses", "text/event-stream", "data: <html>not-a-page\n\n", 200, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			body := `{"model":"synthetic","input":"hi"}`
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				raw, _ := io.ReadAll(r.Body)
				if string(raw) != body || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer synthetic-test" || r.Header.Get("Accept") != "application/json" {
					t.Errorf("request not preserved")
				}
				if len(paths) == 1 {
					w.Header().Set("Content-Type", tc.media)
					w.WriteHeader(tc.status)
					io.WriteString(w, tc.body)
				} else {
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"id":"fallback"}`)
				}
			}))
			defer up.Close()
			client := up.Client()
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			defer client.CloseIdleConnections()
			s := &Server{upstream: client}
			base := up.URL + tc.base
			req, _ := http.NewRequestWithContext(t.Context(), "POST", base+protocol.Endpoint(tc.protocol), strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer synthetic-test")
			req.Header.Set("Accept", "application/json")
			evidence := map[string]string{}
			resp, err := s.inferenceResponse(req, base, tc.protocol, evidence)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)
			wantBody := tc.body
			if tc.calls == 2 {
				wantBody = `{"id":"fallback"}`
			}
			if len(paths) != tc.calls || string(raw) != wantBody {
				t.Fatalf("paths=%v body=%q", paths, raw)
			}
			if tc.calls == 2 {
				if paths[1] != tc.base+"/v1"+protocol.Endpoint(tc.protocol) || evidence["endpoint_resolution"] != "v1_fallback" || evidence["endpoint_requests"] != "2" || evidence["initial_response_kind"] != "html" {
					t.Fatalf("paths=%v evidence=%v", paths, evidence)
				}
			} else if evidence["endpoint_resolution"] != "configured" || evidence["endpoint_requests"] != "1" {
				t.Fatal(evidence)
			}
		})
	}
}

func TestInferenceSniffPreservesBodyAndDoesNotBufferJSON(t *testing.T) {
	for _, tc := range []struct {
		body, media string
		html        bool
	}{
		{" \n<HTML>page</HTML>", "", true},
		{"\ufeff\n<!doctype html>page", "application/json", true},
		{`{"id":"r"}`, "application/json", false},
		{"<not-html>", "text/plain", false},
		{"data: hello\n\n", "", false},
	} {
		resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}
		resp.Header.Set("Content-Type", tc.media)
		if got := inspectInferenceResponse(resp, map[string]string{}); got != tc.html {
			t.Fatalf("html=%v for %q", got, tc.body)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(raw) != tc.body {
			t.Fatalf("body changed: %q", raw)
		}
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	done := make(chan bool, 1)
	go func() {
		done <- inspectInferenceResponse(&http.Response{StatusCode: 200, Header: http.Header{}, Body: reader}, map[string]string{})
	}()
	go func() { writer.Write([]byte("{")) }()
	select {
	case html := <-done:
		if html {
			t.Fatal("JSON marked HTML")
		}
	case <-time.After(time.Second):
		t.Fatal("sniff waited for body completion")
	}
}

func TestInferenceFallbackUsesCallerDeadline(t *testing.T) {
	release := make(chan struct{})
	var mu sync.Mutex
	var paths []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/responses" {
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, "<html>page</html>")
			return
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer up.Close()
	defer close(release)
	client := up.Client()
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", up.URL+"/responses", strings.NewReader("{}"))
	evidence := map[string]string{}
	_, err := (&Server{upstream: client}).inferenceResponse(req, up.URL, "responses", evidence)
	if err == nil || ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("err=%v context=%v", err, ctx.Err())
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(paths, []string{"/responses", "/v1/responses"}) {
		t.Fatal(paths)
	}
}

func TestInferenceHTMLProbeAndGateway(t *testing.T) {
	for _, source := range []string{"probe", "proxy"} {
		for _, mode := range []string{"json", "sse", "html"} {
			t.Run(source+"/"+mode, func(t *testing.T) {
				f := setup(t)
				var mu sync.Mutex
				var paths []string
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					paths = append(paths, r.URL.Path)
					mu.Unlock()
					if r.URL.Path == "/responses" || mode == "html" {
						w.Header().Set("Content-Type", "text/html")
						io.WriteString(w, "<html>private-page synthetic-secret</html>")
						return
					}
					if mode == "sse" {
						w.Header().Set("Content-Type", "text/event-stream")
						io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"OK\"}]}],\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						io.WriteString(w, `{"id":"r","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":3,"output_tokens":1}}`)
					}
				}))
				defer up.Close()
				f.add(t, "p", up.URL, "synthetic-secret")
				if source == "probe" {
					in := catalog.ProbeRequest{ProviderID: "p", ExpectedRevision: 1, Action: "start", KeyID: "default", Model: "upstream", Approved: true}
					initial := probeCall(t, f, in)
					in.Action = "read"
					in.RunID = initial.RunID
					final := awaitProbe(t, f, in, func(p catalog.ProbeProgress) bool { return p.Status == "completed" })
					failure := ""
					if mode == "html" {
						failure = "unexpected_html"
					}
					if final.Result.Failure != failure || final.Result.HTTPStatus != 200 || (mode != "html" && final.Text != "OK") {
						t.Fatalf("result=%+v text=%q", final.Result, final.Text)
					}
				} else {
					f.model(t, state.Candidate{ProviderID: "p", Model: "upstream"})
					payload := `{"model":"public","input":"hi"}`
					if mode == "sse" {
						payload = `{"model":"public","input":"hi","stream":true}`
					}
					resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, payload)
					raw := text(t, resp)
					status := 200
					if mode == "html" {
						status = 502
					}
					if resp.StatusCode != status || (mode == "html" && !strings.Contains(raw, "unexpected_html")) || (mode != "html" && !strings.Contains(raw, "OK")) {
						t.Fatalf("status=%d body=%s", resp.StatusCode, raw)
					}
				}
				mu.Lock()
				got := append([]string(nil), paths...)
				mu.Unlock()
				if !reflect.DeepEqual(got, []string{"/responses", "/v1/responses"}) {
					t.Fatal(got)
				}
				if err := f.core.logs.Flush(t.Context()); err != nil {
					t.Fatal(err)
				}
				logs, err := f.core.logs.List(t.Context(), logbook.Query{Source: source})
				if err != nil || len(logs.Requests) != 1 {
					t.Fatalf("logs=%+v err=%v", logs, err)
				}
				row, err := f.core.logs.Detail(t.Context(), logs.Requests[0].ID)
				if err != nil {
					t.Fatal(err)
				}
				if row.Evidence["endpoint_resolution"] != "v1_fallback" || row.Evidence["endpoint_requests"] != "2" || row.AttemptCount != 1 {
					t.Fatalf("row=%+v", row)
				}
				if mode == "html" && (row.Code != "unexpected_html" || row.Attempts[0].HTTP == nil || *row.Attempts[0].HTTP != 200) {
					t.Fatalf("row=%+v", row)
				}
				raw, _ := json.Marshal(row)
				if strings.Contains(string(raw), "private-page") || strings.Contains(string(raw), "synthetic-secret") {
					t.Fatal("private response leaked into logs")
				}
				p, _ := f.store.Provider("p")
				if p.BaseURL != up.URL || p.Revision != 1 {
					t.Fatalf("configuration changed: %+v", p)
				}
			})
		}
	}
}
