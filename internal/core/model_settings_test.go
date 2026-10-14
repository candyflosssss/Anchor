package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/candyflosssss/Anchor/internal/catalog"
	"github.com/candyflosssss/Anchor/internal/client"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func TestModelSettingsScopedSaveAndSnapshot(t *testing.T) {
	f := setup(t)
	f.add(t, "p", "http://127.0.0.1:1/v1", "synthetic")
	m, e := f.store.PutModel(state.Model{ID: "中文模型", Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "actual", Multiplier: 2, KeyIDs: []string{"default"}}}}, 0)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile("../modeldev/testdata/catalog.json")
	if e != nil {
		t.Fatal(e)
	}
	var calls atomic.Int32
	f.core.modelCatalog = modeldev.NewWithTransport(modelCatalogTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(raw))}, nil
	}))
	settings, _ := f.store.Settings()
	cl, _, e := client.Connect(context.Background(), state.Context{Name: "test", Endpoint: f.control.URL, InstanceID: settings.InstanceID, CredentialRef: "local-owner"}, f.owner)
	if e != nil {
		t.Fatal(e)
	}
	defer cl.Close()
	current, e := cl.ModelSettings(context.Background(), catalog.ModelSettingsQuery{ModelID: m.ID})
	if e != nil || current.Revision != 1 || !*current.Configuration.Capabilities.ToolCall {
		t.Fatal(current, e)
	}
	n := int64(272000)
	current.Configuration.Limits.Context = &n
	in := catalog.SaveModelSettings{ModelID: m.ID, ExpectedRevision: 1, DisplayName: "日常对话模型", Configuration: current.Configuration, ReferenceAction: "set", ReferenceProviderID: "lab", ReferenceModelID: "vision-reasoner"}
	out, e := cl.SaveModelSettings(context.Background(), in)
	if e != nil || out.Revision != 2 || out.Reference == nil || out.Reference.Models[0].Pricing == nil || calls.Load() != 1 {
		t.Fatal(out, e, calls.Load())
	}
	persisted, _ := f.store.Model(m.ID)
	if !reflect.DeepEqual(persisted.Candidates, m.Candidates) || persisted.Settings == nil || *persisted.Settings.Configuration.Limits.Context != n {
		t.Fatal(persisted)
	}
	_, e = cl.SaveModelSettings(context.Background(), in)
	if e == nil || calls.Load() != 1 {
		t.Fatal("stale write accepted", e, calls.Load())
	}
	in.ExpectedRevision = 2
	in.ReferenceAction = "keep"
	in.ReferenceProviderID = ""
	in.ReferenceModelID = ""
	out, e = cl.SaveModelSettings(context.Background(), in)
	if e != nil || out.Reference.Version == "" || calls.Load() != 1 {
		t.Fatal(out, e)
	}
	// Older full mapping writers retain scoped metadata and reference snapshots.
	old := m
	old.DisplayName = "legacy"
	old.Tools = false
	updated, e := cl.PutModel(context.Background(), old, 3)
	if e != nil || updated.Settings == nil || updated.Settings.Reference == nil || *updated.Settings.Configuration.Limits.Context != n {
		t.Fatal(updated, e)
	}
	read, e := cl.ModelSettings(context.Background(), catalog.ModelSettingsQuery{ModelID: m.ID})
	if e != nil || *read.Configuration.Capabilities.ToolCall {
		t.Fatal(read, e)
	}
	in.ExpectedRevision = 4
	in.ReferenceAction = "clear"
	out, e = cl.SaveModelSettings(context.Background(), in)
	if e != nil || out.Reference != nil || calls.Load() != 1 {
		t.Fatal(out, e)
	}
	// Boundary rejects absent ownership/identity and caller-supplied snapshots.
	for _, tc := range []struct {
		token, identity, body string
		status                int
	}{
		{"", settings.InstanceID, "{}", 401}, {f.token, settings.InstanceID, "{}", 401}, {f.owner, "wrong", "{}", 409},
		{f.owner, settings.InstanceID, `{"model_id":"中文模型","expected_revision":5,"reference_action":"keep","reference":{"source":"models.dev"}}`, 400},
	} {
		req, e := http.NewRequest("POST", f.control.URL+"/control/v1/models/"+url.PathEscape(m.ID)+"/settings", strings.NewReader(tc.body))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("X-Anchor-Instance", tc.identity)
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("status=%d want=%d %s", resp.StatusCode, tc.status, body)
		}
	}
	if _, e := json.Marshal(out); e != nil {
		t.Fatal(e)
	}
}
