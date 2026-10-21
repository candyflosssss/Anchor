package core

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/providerapi"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestPlatformDetectionCreationCacheAndManualRefresh(t *testing.T) {
	f := setup(t)
	var probes atomic.Int32
	var changed atomic.Bool
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/status":
			probes.Add(1)
			if r.Header.Get("Authorization") != "" {
				t.Error("public probe carried credentials")
			}
			if changed.Load() {
				io.WriteString(w, `{"data":{"version":"1","quota_per_unit":500000,"oidc":false,"oidc_well_known":""}}`)
				return
			}
			w.WriteHeader(404)
		case "/api/v1/settings/public":
			probes.Add(1)
			io.WriteString(w, `{"data":{"registration_enabled":true,"site_name":"fixture","version":"1","api_base_url":""}}`)
		case "/v1/usage":
			io.WriteString(w, `{"mode":"unrestricted","balance":12}`)
		default:
			t.Error("unexpected endpoint", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer up.Close()
	cfg, _ := f.store.Settings()
	body := `{"provider":{"id":"created","base_url":"` + up.URL + `/v1","enabled":false,"timeout_seconds":5,"max_concurrent":1},"expected_revision":0,"secret":"synthetic-detection-key"}`
	req, _ := http.NewRequest("PUT", f.control.URL+"/control/v1/providers/created", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.owner)
	req.Header.Set("X-Anchor-Instance", cfg.InstanceID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || probes.Load() != 2 {
		t.Fatal(resp.StatusCode, probes.Load())
	}
	settings, err := f.store.ProviderPlatformSettings("created")
	if err != nil || settings.Config.DetectedPlatform != "sub2api" || settings.Revision != 1 {
		t.Fatal(settings, err)
	}
	changed.Store(true)
	q := providerapi.Query{ProviderID: "created", ExpectedRevision: 1, Operation: "key-quota"}
	for range 3 {
		v, e := f.core.QueryProvider(t.Context(), q)
		if e != nil || v.Platform != "sub2api" || v.Failure != "" {
			t.Fatal(v, e)
		}
	}
	if probes.Load() != 2 {
		t.Fatal("normal reads repeated detection")
	}
	q.Operation = "detect-platform"
	v, err := f.core.QueryProvider(t.Context(), q)
	if err != nil || v.Platform != "oneapi" || probes.Load() != 3 {
		t.Fatal(v, err, probes.Load())
	}
	settings, _ = f.store.ProviderPlatformSettings("created")
	if settings.Config.DetectedPlatform != "oneapi" || settings.Revision != 1 {
		t.Fatal(settings)
	}
}

func TestFailedPlatformDetectionIsFrozenUntilManualRefresh(t *testing.T) {
	f := setup(t)
	var requests atomic.Int32
	var available atomic.Bool
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if available.Load() && r.URL.Path == "/api/v1/settings/public" {
			io.WriteString(w, `{"data":{"registration_enabled":true,"site_name":"fixture","version":"1","api_base_url":""}}`)
			return
		}
		w.WriteHeader(404)
	}))
	defer up.Close()
	f.add(t, "p", up.URL+"/v1", "synthetic-detection-key")
	q := providerapi.Query{ProviderID: "p", ExpectedRevision: 1, Operation: "balance-summary"}
	v, err := f.core.QueryProvider(t.Context(), q)
	if err != nil || v.Failure != "platform_unknown" || v.Operation != q.Operation {
		t.Fatal(v, err)
	}
	count := requests.Load()
	available.Store(true)
	for range 3 {
		v, err = f.core.QueryProvider(t.Context(), q)
		if err != nil || v.Failure != "platform_unknown" {
			t.Fatal(v, err)
		}
	}
	if requests.Load() != count {
		t.Fatal("failed detection retried automatically")
	}
	q.Operation = "detect-platform"
	v, err = f.core.QueryProvider(t.Context(), q)
	if err != nil || v.Failure != "" || v.Platform != "sub2api" || requests.Load() != count+2 {
		t.Fatal(v, err)
	}
}

func TestConcurrentPlatformDetectionAndStaleResult(t *testing.T) {
	f := setup(t)
	entered, release := make(chan struct{}), make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		io.WriteString(w, `{"data":{"version":"1","quota_per_unit":500000,"default_use_auto_group":true,"enable_batch_update":false}}`)
	}))
	defer up.Close()
	f.add(t, "p", up.URL+"/v1", "synthetic-detection-key")
	q := providerapi.Query{ProviderID: "p", ExpectedRevision: 1, Operation: "detect-platform"}
	done := make(chan error, 1)
	go func() { _, err := f.core.QueryProvider(context.Background(), q); done <- err }()
	<-entered
	v, err := f.core.QueryProvider(t.Context(), q)
	if err != nil || v.Failure != "busy" {
		close(release)
		t.Fatal(v, err)
	}
	p, _ := f.store.Provider("p")
	p.Enabled = false
	if _, err = f.store.PutProvider(p, 1, nil); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	if err = <-done; !errors.Is(err, state.ErrConflict) {
		t.Fatal("stale detection was saved", err)
	}
	cfg, _ := f.store.ProviderPlatform("p")
	if cfg.DetectedPlatform != "" || cfg.DetectionFailure != "" {
		t.Fatal(cfg)
	}
}
