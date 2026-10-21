package core

import (
	"encoding/json"
	"errors"
	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPriorityKeysFailoverDownweightAndUsage(t *testing.T) {
	f := setup(t)
	var bad, good atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer synthetic-bad" {
			bad.Add(1)
			w.WriteHeader(401)
			io.WriteString(w, "private upstream detail")
			return
		}
		good.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","output":[],"usage":{"input_tokens":10,"output_tokens":2}}`)
	}))
	defer upstream.Close()
	f.add(t, "p", upstream.URL, "synthetic-bad")
	if err := f.store.MutateProvider("p", 1, "key-put", "second", "synthetic-good", "", true, false); err != nil {
		t.Fatal(err)
	}
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	for i := 0; i < 2; i++ {
		r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
		if r.StatusCode != 200 || strings.Contains(text(t, r), "private") {
			t.Fatal("failover failed")
		}
	}
	if bad.Load() != 1 || good.Load() != 2 {
		t.Fatal(bad.Load(), good.Load())
	}
	w, err := f.core.workspace("p")
	if err != nil || len(w.Keys) != 2 || w.Keys[0].State != "cooling" || len(w.Usage) != 1 || w.Usage[0].Requests != 3 || w.Usage[0].Known != 2 {
		t.Fatal(w, err)
	}
	p, _ := f.store.Provider("p")
	f.core.finishKey(p, "second", 403, "120", false)
	if _, ok := f.core.chooseKey(p, nil); ok {
		t.Fatal("cooling keys selected")
	}
	f.core.mu.Lock()
	f.core.health[healthID(p, "second")].Until = time.Now().Add(-time.Second)
	f.core.mu.Unlock()
	k, ok := f.core.chooseKey(p, nil)
	if !ok || k.ID != "second" {
		t.Fatal(k, ok)
	}
	if _, ok = f.core.chooseKey(p, nil); !ok {
		t.Fatal("exclusive probe blocked a request")
	}
	f.core.finishKey(p, k.ID, 200, "", true)
	if _, ok = f.core.chooseKey(p, nil); !ok {
		t.Fatal("success did not recover")
	}
}
func TestPriorityManagementActions(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1/v1", "synthetic-key")
	cfg, _ := f.store.Settings()
	act := func(a catalog.Action) (int, catalog.Workspace) {
		t.Helper()
		body, _ := json.Marshal(a)
		req, _ := http.NewRequest("POST", f.control.URL+"/control/v1/providers/p/action", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+f.owner)
		req.Header.Set("X-Anchor-Instance", cfg.InstanceID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(raw), "synthetic-key") {
			t.Fatal("secret leaked")
		}
		var w catalog.Workspace
		json.Unmarshal(raw, &w)
		return resp.StatusCode, w
	}
	if status, _ := act(catalog.Action{ProviderID: "p", ExpectedRevision: 2, Action: "key-put", Target: "second", Secret: "synthetic-two", Enabled: true}); status != 409 {
		t.Fatal(status)
	}
	if status, w := act(catalog.Action{ProviderID: "p", ExpectedRevision: 1, Action: "model-put", Target: "alias", Model: "up", Tools: true, Streaming: true}); status != 200 || len(w.Models) != 1 {
		t.Fatal(status, w)
	}
	if status, _ := act(catalog.Action{ProviderID: "p", ExpectedRevision: 2, Action: "delete", Approved: true}); status != 409 {
		t.Fatal(status)
	}
	if status, w := act(catalog.Action{ProviderID: "p", ExpectedRevision: 1, Action: "price-put", Target: "alias", Price: &catalog.Price{InputUSD: 1, OutputUSD: 3}}); status != 200 || w.Revision != 2 || len(w.Prices) != 1 {
		t.Fatal(status, w)
	}
	f.store.RecordUsage(state.Usage{ProviderID: "p", Model: "alias", Requests: 2, Known: 1, InputTokens: 1000000, OutputTokens: 1000000})
	w, _ := f.core.workspace("p")
	if len(w.Estimates) != 1 || w.Estimates[0].USD == nil || *w.Estimates[0].USD != 4 {
		t.Fatal(w.Estimates)
	}
	price, err := parsePrice([]byte(`{"vendor":{"models":{"up":{"cost":{"input":0,"output":2.5,"cache_read":0.1}}}}}`), "alias", "vendor", "up")
	if err != nil || price.InputUSD != 0 || price.CacheWriteUSD != nil || price.CacheReadUSD == nil {
		t.Fatal(price, err)
	}
	if status, deleted := act(catalog.Action{ProviderID: "p", ExpectedRevision: 2, Action: "delete", Approved: true}); status != 200 || !deleted.Deleted || len(deleted.Models) != 0 || len(deleted.Keys) != 0 || len(deleted.Prices) != 0 {
		t.Fatal(status, deleted)
	}
	if _, err = f.store.Model("alias"); !errors.Is(err, state.ErrNotFound) {
		t.Fatal("orphan retained", err)
	}
}
func TestPriorityProtocolGatewayAndProbe(t *testing.T) {
	for _, p := range []string{"chat-completions", "anthropic"} {
		t.Run(p, func(t *testing.T) {
			f := setup(t)
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var m map[string]any
				json.NewDecoder(r.Body).Decode(&m)
				if m["model"] != "up" || m["messages"] == nil {
					t.Error("request conversion")
				}
				w.Header().Set("Content-Type", "application/json")
				if p == "anthropic" {
					if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "synthetic" {
						t.Error("anthropic auth/path")
					}
					io.WriteString(w, `{"id":"r","content":[{"type":"text","text":"OK"}],"usage":{"input_tokens":3,"output_tokens":1},"stop_reason":"end_turn"}`)
				} else {
					io.WriteString(w, `{"id":"r","choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
				}
			}))
			defer up.Close()
			f.add(t, "p", up.URL+"/v1", "synthetic")
			provider, _ := f.store.Provider("p")
			err := f.store.MutateProvider(provider.ID, provider.Revision, "key-put", "default", "", "", true, false, p)
			provider, _ = f.store.Provider(provider.ID)
			if err != nil {
				t.Fatal(err)
			}
			f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
			r := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
			raw := text(t, r)
			if r.StatusCode != 200 || !strings.Contains(raw, `"output_text"`) || !strings.Contains(raw, `"input_tokens":3`) {
				t.Fatal(r.StatusCode, raw)
			}
			probe, err := f.core.probe(t.Context(), provider, "", "up")
			if err != nil || probe.Failure != "" || probe.HTTPStatus != 200 {
				t.Fatal(probe, err)
			}
		})
	}
}

func TestChineseModelAndKeyGatewayRouting(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "upstream/model" || r.Header.Get("Authorization") != "Bearer synthetic-chinese-key" {
			t.Error("wrong routed model or credential")
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","output":[],"usage":{"input_tokens":10,"output_tokens":2}}`)
	}))
	defer upstream.Close()
	f.add(t, "中文供应商", upstream.URL, "synthetic-unused")
	if err := f.store.MutateProvider("中文供应商", 1, "key-put", "主力密钥", "synthetic-chinese-key", "", true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.PutModel(state.Model{ID: "中文模型", Candidates: []state.Candidate{{ProviderID: "中文供应商", Model: "upstream/model", KeyIDs: []string{"主力密钥"}}}}, 0); err != nil {
		t.Fatal(err)
	}
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"中文模型","input":"你好"}`)
	body := text(t, resp)
	if resp.StatusCode != 200 || calls.Load() != 1 {
		t.Fatal(resp.StatusCode, body, calls.Load())
	}
}
