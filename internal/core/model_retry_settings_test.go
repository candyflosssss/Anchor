package core

import (
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/state"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLegacyMappingRetryLimitDoesNotReplay(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	var model state.Model
	if err := json.Unmarshal([]byte(`{"id":"public","candidates":[{"provider_id":"p","model":"up"}],"settings":{"channel_retries":10}}`), &model); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.PutModel(model, 0); err != nil {
		t.Fatal(err)
	}
	resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public"}`)
	body := text(t, resp)
	if resp.StatusCode != 503 || calls.Load() != 1 {
		t.Fatal(resp.StatusCode, calls.Load(), body)
	}
	cfg, _ := f.store.Settings()
	req, _ := http.NewRequest("POST", f.control.URL+"/control/v1/models/public/settings", strings.NewReader(`{"model_id":"public","expected_revision":1,"channel_retries":3}`))
	req.Header.Set("Authorization", "Bearer "+f.owner)
	req.Header.Set("X-Anchor-Instance", cfg.InstanceID)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	io.Copy(io.Discard, res.Body)
	if res.StatusCode != 400 {
		t.Fatal("retired field silently accepted", res.StatusCode)
	}
}
