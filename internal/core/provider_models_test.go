package core

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/candyflosssss/Anchor/internal/catalog"
)

func discover(t *testing.T, f *fixture, id, body, token, identity string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", f.control.URL+"/control/v1/providers/"+id+"/discover-models", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Anchor-Instance", identity)
	client := &http.Client{Timeout: 4 * time.Second}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(raw)
}
func TestProviderDirectoryBoundariesAndReadOnly(t *testing.T) {
	f := setup(t)
	cfg, _ := f.store.Settings()
	var calls atomic.Int32
	secret := "synthetic-catalog-secret"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("unexpected upstream request")
		}
		io.WriteString(w, `{"data":[{"id":"z"},{"id":"a","owner":"discard"},{"id":"a"}]}`)
	}))
	defer upstream.Close()
	f.add(t, "中文供应商", upstream.URL+"/v1/", secret)
	p, _ := f.store.Provider("中文供应商")
	p.Enabled = false
	p, err := f.store.PutProvider(p, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, _ = f.store.Provider(p.ID) // Compare the internal snapshot, including the stored secret.
	body := `{"provider_id":"中文供应商","expected_revision":2}`
	for _, tc := range []struct {
		body, token, identity string
		status                int
	}{
		{body, "", cfg.InstanceID, 401}, {body, f.token, cfg.InstanceID, 401},
		{body, f.owner, "wrong", 409},
		{`{"provider_id":"中文供应商","expected_revision":1}`, f.owner, cfg.InstanceID, 409},
		{`{"provider_id":"other","expected_revision":2}`, f.owner, cfg.InstanceID, 400},
		{`{"provider_id":"中文供应商","expected_revision":0}`, f.owner, cfg.InstanceID, 400},
		{`{"provider_id":"中文供应商","expected_revision":2,"url":"https://untrusted.invalid"}`, f.owner, cfg.InstanceID, 400},
	} {
		status, raw := discover(t, f, p.ID, tc.body, tc.token, tc.identity)
		if status != tc.status || strings.Contains(raw, secret) {
			t.Fatalf("status=%d want=%d", status, tc.status)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("upstream called before validation")
	}
	status, raw := discover(t, f, p.ID, body, f.owner, cfg.InstanceID)
	var result catalog.ProviderModels
	if status != 200 || json.Unmarshal([]byte(raw), &result) != nil || result.Failure != "" || !reflect.DeepEqual(result.Models, []string{"a", "z"}) || result.Revision != 2 || result.HTTPStatus != 200 {
		t.Fatal(status, raw)
	}
	after, _ := f.store.Provider(p.ID)
	if !reflect.DeepEqual(p, after) {
		t.Fatal("discovery mutated provider")
	}
	mappings, _ := f.store.Models()
	if len(mappings) != 0 {
		t.Fatal("imported mappings")
	}
	if calls.Load() != 1 || strings.Contains(raw, secret) || strings.Contains(f.logs.String(), secret) {
		t.Fatal("retry or secret leak")
	}
}
func TestProviderDirectoryFailures(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		body, failure string
	}{
		{"auth", 401, "synthetic-catalog-secret", "authentication"},
		{"unsupported", 404, "raw error", "unsupported"}, {"rate", 429, "raw error", "rate_limited"},
		{"server", 503, "raw error", "http_error"},
		{"redirect", 302, "raw error", "http_error"},
		{"bad", 200, `{"data":null}`, "invalid_response"},
		{"secret-id", 200, `{"data":[{"id":"synthetic-catalog-secret"}]}`, "invalid_response"},
		{"large", 200, strings.Repeat("x", catalog.MaxBytes+1), "too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			cfg, _ := f.store.Settings()
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer upstream.Close()
			f.add(t, "p", upstream.URL, "synthetic-catalog-secret")
			status, raw := discover(t, f, "p", `{"provider_id":"p","expected_revision":1}`, f.owner, cfg.InstanceID)
			var result catalog.ProviderModels
			if status != 200 || json.Unmarshal([]byte(raw), &result) != nil || result.Failure != tc.failure || result.HTTPStatus != tc.status || result.Models == nil || len(result.Models) != 0 || calls.Load() != 1 {
				t.Fatal(status, raw, calls.Load())
			}
			if strings.Contains(raw, "synthetic-catalog-secret") || strings.Contains(raw, "raw error") {
				t.Fatal("reflected upstream body")
			}
		})
	}
}
func TestProviderDirectoryConcurrencyAndTimeout(t *testing.T) {
	f := setup(t)
	cfg, _ := f.store.Settings()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); <-r.Context().Done() }))
	defer upstream.Close()
	f.add(t, "p", upstream.URL, "")
	p, _ := f.store.Provider("p")
	p.TimeoutSeconds = 1
	p, _ = f.store.PutProvider(p, 1, nil)
	body := `{"provider_id":"p","expected_revision":2}`
	check := func(want string) {
		t.Helper()
		status, raw := discover(t, f, "p", body, f.owner, cfg.InstanceID)
		var result catalog.ProviderModels
		_ = json.Unmarshal([]byte(raw), &result)
		if status != 200 || result.Failure != want {
			t.Fatal(status, raw)
		}
	}
	f.core.take("p", 1)
	check("busy")
	f.core.release("p")
	for i := 0; i < cap(f.core.slots); i++ {
		f.core.slots <- struct{}{}
	}
	check("busy")
	for i := 0; i < cap(f.core.slots); i++ {
		<-f.core.slots
	}
	if calls.Load() != 0 {
		t.Fatal("busy request called upstream")
	}
	check("timeout")
	if calls.Load() != 1 {
		t.Fatal("timeout replayed")
	}
	if !f.core.take("p", 1) || len(f.core.slots) != 0 {
		t.Fatal("slots not released")
	}
	f.core.release("p")
}

func TestProviderDirectoryNetworkFailure(t *testing.T) {
	f := setup(t)
	cfg, _ := f.store.Settings()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("closed upstream called") }))
	base := upstream.URL
	upstream.Close()
	f.add(t, "p", base, "synthetic-catalog-secret")
	status, raw := discover(t, f, "p", `{"provider_id":"p","expected_revision":1}`, f.owner, cfg.InstanceID)
	var result catalog.ProviderModels
	if status != 200 || json.Unmarshal([]byte(raw), &result) != nil || result.Failure != "network" || result.HTTPStatus != 0 || len(result.Models) != 0 || strings.Contains(raw, "synthetic-catalog-secret") || strings.Contains(raw, base) {
		t.Fatal(status, raw)
	}
}
