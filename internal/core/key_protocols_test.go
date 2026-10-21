package core

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestModelOverrideProtocolConversionMatrix(t *testing.T) {
	for _, upstreamProtocol := range []string{"responses", "chat-completions", "anthropic"} {
		for _, downstreamProtocol := range []string{"responses", "chat-completions", "anthropic"} {
			t.Run(upstreamProtocol+"/"+downstreamProtocol, func(t *testing.T) {
				f := setup(t)
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/v1"+protocol.Endpoint(upstreamProtocol) {
						t.Errorf("path=%s", r.URL.Path)
					}
					if upstreamProtocol == "anthropic" {
						if r.Header.Get("x-api-key") != "synthetic" || r.Header.Get("Authorization") != "" {
							t.Error("anthropic authentication")
						}
					} else if r.Header.Get("Authorization") != "Bearer synthetic" || r.Header.Get("x-api-key") != "" {
						t.Error("openai authentication")
					}
					var m map[string]any
					if json.NewDecoder(r.Body).Decode(&m) != nil || m["model"] != "actual/model" {
						t.Error("actual upstream model")
					}
					w.Header().Set("Content-Type", "application/json")
					switch upstreamProtocol {
					case "responses":
						io.WriteString(w, `{"id":"r","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":3,"output_tokens":1}}`)
					case "chat-completions":
						io.WriteString(w, `{"id":"r","choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
					case "anthropic":
						io.WriteString(w, `{"id":"r","content":[{"type":"text","text":"OK"}],"usage":{"input_tokens":3,"output_tokens":1},"stop_reason":"end_turn"}`)
					}
				}))
				defer up.Close()
				f.add(t, "p", up.URL+"/v1", "synthetic")
				// The provider and key default deliberately differ from the override.
				if err := f.store.MutateProvider("p", 1, "key-put", "default", "", "", true, false, "chat-completions"); err != nil {
					t.Fatal(err)
				}
				if err := f.store.PutKeyModelProtocol("p", 2, "default", "actual/model", upstreamProtocol); err != nil {
					t.Fatal(err)
				}
				f.model(t, state.Candidate{ProviderID: "p", Model: "actual/model"})
				body := `{"model":"public","input":"hi"}`
				if downstreamProtocol != "responses" {
					body = `{"model":"public","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`
				}
				response := request(t, "POST", f.gateway.URL+"/v1"+protocol.Endpoint(downstreamProtocol), f.token, body)
				raw := text(t, response)
				if response.StatusCode != 200 || !strings.Contains(raw, "OK") || (upstreamProtocol != downstreamProtocol && !strings.Contains(raw, "public")) {
					t.Fatal(response.StatusCode, raw)
				}
				p, _ := f.store.Provider("p")
				probe, err := f.core.probe(t.Context(), p, "default", "actual/model")
				if err != nil || probe.HTTPStatus != 200 || probe.Failure != "" {
					t.Fatal(probe, err)
				}
			})
		}
	}
}

func TestKeyFailoverResolvesEachProtocol(t *testing.T) {
	f := setup(t)
	var paths []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/chat/completions" && r.Header.Get("Authorization") == "Bearer synthetic-bad" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "synthetic-good" {
			t.Error("second key's model override not used")
		}
		io.WriteString(w, `{"id":"r","content":[{"type":"text","text":"OK"}],"usage":{"input_tokens":1,"output_tokens":1},"stop_reason":"end_turn"}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL+"/v1", "synthetic-bad")
	if err := f.store.MutateProvider("p", 1, "key-put", "default", "", "", true, false, "chat-completions"); err != nil {
		t.Fatal(err)
	}
	if err := f.store.MutateProvider("p", 2, "key-put", "second", "synthetic-good", "", true, false, "responses"); err != nil {
		t.Fatal(err)
	}
	if err := f.store.PutKeyModelProtocol("p", 3, "second", "up", "anthropic"); err != nil {
		t.Fatal(err)
	}
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	response := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
	if raw := text(t, response); response.StatusCode != 200 || !strings.Contains(raw, "OK") {
		t.Fatal(response.StatusCode, raw)
	}
	if !reflect.DeepEqual(paths, []string{"/v1/chat/completions", "/v1/messages"}) {
		t.Fatal(paths)
	}
}

func TestDirectoryUsesSelectedKeyDefaultProtocol(t *testing.T) {
	f := setup(t)
	cfg, _ := f.store.Settings()
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/v1/sub2api/billing" {
			if r.Header.Get("Authorization") != "Bearer synthetic-second" || r.Header.Get("x-api-key") != "" {
				t.Error("billing must use bearer independently of inference protocol")
			}
			w.WriteHeader(404)
			return
		}
		if r.URL.Path != "/v1/models" || r.Header.Get("x-api-key") != "synthetic-second" || r.Header.Get("Authorization") != "" {
			t.Error("wrong catalog credential/protocol")
		}
		io.WriteString(w, `{"data":[{"id":"actual/model"}]}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL+"/v1", "synthetic-default")
	savedCatalogPlatform(t, f, "p")
	if err := f.store.MutateProvider("p", 1, "key-put", "second", "synthetic-second", "", true, false, "anthropic"); err != nil {
		t.Fatal(err)
	}
	if err := f.store.PutKeyModelProtocol("p", 2, "second", "actual/model", "responses"); err != nil {
		t.Fatal(err)
	}
	before, _ := f.store.ProviderKeys("p")
	status, raw := discover(t, f, "p", `{"provider_id":"p","expected_revision":3,"key_id":"second"}`, f.owner, cfg.InstanceID)
	var result catalog.ProviderModels
	if status != 200 || json.Unmarshal([]byte(raw), &result) != nil || result.KeyID != "second" || result.Failure != "" || calls != 2 {
		t.Fatal(status, raw)
	}
	status, _ = discover(t, f, "p", `{"provider_id":"p","expected_revision":3,"key_id":"missing"}`, f.owner, cfg.InstanceID)
	if status != 404 || calls != 2 {
		t.Fatal(status, calls)
	}
	after, _ := f.store.ProviderKeys("p")
	for i := range after {
		after[i].Rate = nil
	}
	p, _ := f.store.Provider("p")
	if !reflect.DeepEqual(before, after) || p.Revision != 3 {
		t.Fatal("discovery changed settings")
	}
}

func TestStreamingUsesActualModelProtocol(t *testing.T) {
	f := setup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "synthetic" {
			t.Error("stream did not use model protocol")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`{"type":"message_start","message":{"id":"r","usage":{"input_tokens":2,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"OK"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`,
			`{"type":"message_stop"}`,
		} {
			io.WriteString(w, "data: "+event+"\n\n")
		}
	}))
	defer up.Close()
	f.add(t, "p", up.URL+"/v1", "synthetic")
	if err := f.store.PutKeyModelProtocol("p", 1, "default", "up", "anthropic"); err != nil {
		t.Fatal(err)
	}
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi","stream":true}`)
	raw := text(t, r)
	if r.StatusCode != 200 || !strings.Contains(raw, "OK") || !strings.Contains(raw, "response.completed") || strings.Contains(raw, "upstream_stream_error") {
		t.Fatal(r.StatusCode, raw)
	}
	w, err := f.core.workspace("p")
	if err != nil || len(w.Usage) != 1 || w.Usage[0].Known != 1 || w.Usage[0].InputTokens != 2 {
		t.Fatal(w, err)
	}
}
