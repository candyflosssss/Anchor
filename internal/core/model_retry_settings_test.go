package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestModelRetrySettingsPersistAndPreserve(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1", "synthetic")
	f.model(t, state.Candidate{ProviderID: "p", Model: "up"})
	cl := defaultsClient(t, f)
	cfg, _ := f.store.Settings()
	post := func(body map[string]any) (int, []byte) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", f.control.URL+"/control/v1/models/public/settings", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+f.owner)
		req.Header.Set("X-Anchor-Instance", cfg.InstanceID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, data
	}
	out, err := cl.ModelSettings(t.Context(), catalog.ModelSettingsQuery{ModelID: "public"})
	if err != nil || out.ChannelRetries == nil || *out.ChannelRetries != 3 || out.Revision != 1 {
		t.Fatal(out, err)
	}
	for _, n := range []int{0, 10, 2} {
		in := catalog.SaveModelSettings{ModelID: "public", ExpectedRevision: out.Revision, ChannelRetries: &n, Configuration: out.Configuration, ReferenceAction: "keep"}
		out, err = cl.SaveModelSettings(t.Context(), in)
		if err != nil || out.ChannelRetries == nil || *out.ChannelRetries != n || out.Revision != in.ExpectedRevision+1 {
			t.Fatal(out, err)
		}
		stored, err := f.store.Model("public")
		if err != nil || stored.Settings.ChannelRetries == nil || stored.Settings.RetryLimit() != n {
			t.Fatal(stored, err)
		}
		// Legacy clients and explicit null keep the limit, even when clearing the catalog reference.
		in.ExpectedRevision = out.Revision
		in.ChannelRetries = nil
		in.ReferenceAction = "clear"
		out, err = cl.SaveModelSettings(t.Context(), in)
		if err != nil || out.ChannelRetries == nil || *out.ChannelRetries != n {
			t.Fatal(out, err)
		}
		in.ExpectedRevision = out.Revision
		raw, _ := json.Marshal(in)
		var body map[string]any
		json.Unmarshal(raw, &body)
		body["channel_retries"] = nil
		code, data := post(body)
		if code != 200 || json.Unmarshal(data, &out) != nil || out.ChannelRetries == nil || *out.ChannelRetries != n {
			t.Fatal(code, string(data))
		}
		if _, err := cl.SaveModelSettings(t.Context(), in); err == nil {
			t.Fatal("accepted stale settings")
		}
	}
	for _, invalid := range []any{-1, 11, 1.5, "3", true} {
		body := map[string]any{"model_id": "public", "expected_revision": out.Revision, "channel_retries": invalid, "configuration": out.Configuration, "reference_action": "keep"}
		if code, data := post(body); code != 400 {
			t.Fatal(invalid, code, string(data))
		}
	}
	reopened, err := state.Open(f.store.Dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	stored, err := reopened.Model("public")
	if err != nil || stored.Settings.RetryLimit() != 2 || stored.Revision != out.Revision {
		t.Fatal(stored, err)
	}
	// An ordinary mapping update from an older client must retain retry settings.
	stored.Settings = nil
	stored, err = cl.PutModel(t.Context(), stored, stored.Revision)
	if err != nil || stored.Settings.RetryLimit() != 2 {
		t.Fatal(stored, err)
	}
	if _, err := cl.SaveModelSettings(t.Context(), catalog.SaveModelSettings{ModelID: "public", ExpectedRevision: stored.Revision - 1, ReferenceAction: "keep"}); err == nil {
		t.Fatal("stale write accepted")
	}
	if stored.Settings.RetryLimit() != 2 {
		t.Fatal("mutated stored settings")
	}
}

func TestMappingChannelRetryLimitControlsGateway(t *testing.T) {
	for _, n := range []int{0, 1, 2, 4} {
		for _, recover := range []bool{false, true} {
			t.Run(fmt.Sprintf("retries=%d/recover=%t", n, recover), func(t *testing.T) {
				f := setup(t)
				var calls atomic.Int32
				up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					attempt := calls.Add(1)
					if !recover || attempt <= int32(n) {
						w.WriteHeader(503)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"id":"r","status":"completed","output":[]}`)
				}))
				defer up.Close()
				f.add(t, "p", up.URL, "synthetic")
				_, err := f.store.PutModel(state.Model{ID: "public", Candidates: []state.Candidate{{ProviderID: "p", Model: "up"}}, Settings: &state.ModelSettings{ChannelRetries: &n}}, 0)
				if err != nil {
					t.Fatal(err)
				}
				resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
				body := text(t, resp)
				want := 503
				if recover {
					want = 200
				}
				if resp.StatusCode != want || calls.Load() != int32(n+1) {
					t.Fatal(resp.StatusCode, calls.Load(), body)
				}
			})
		}
	}
}

func TestConfigurableRetryBackoffCapsAtOneSecond(t *testing.T) {
	for i, want := range []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, time.Second} {
		if got := channelRetryDelay(i); got != want {
			t.Fatal(i, got, want)
		}
	}
	if channelRetryDelay(1000) != time.Second {
		t.Fatal("backoff overflow")
	}
}

func TestModelRetrySettingsApplyToNextRequest(t *testing.T) {
	f := setup(t)
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := calls.Add(1)
		if attempt == 1 {
			m, err := f.store.Model("public")
			if err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			zero := 0
			m.Settings = &state.ModelSettings{ChannelRetries: &zero}
			if _, err := f.store.PutModel(m, m.Revision); err != nil {
				t.Error(err)
			}
		}
		if attempt != 2 {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"r","status":"completed","output":[]}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL, "synthetic")
	one := 1
	if _, err := f.store.PutModel(state.Model{ID: "public", Candidates: []state.Candidate{{ProviderID: "p", Model: "up"}}, Settings: &state.ModelSettings{ChannelRetries: &one}}, 0); err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{200, 503} {
		resp := request(t, "POST", f.gateway.URL+"/v1/responses", f.token, `{"model":"public","input":"hi"}`)
		body := text(t, resp)
		if resp.StatusCode != want || calls.Load() != int32(i+2) {
			t.Fatal(resp.StatusCode, calls.Load(), body)
		}
	}
}
