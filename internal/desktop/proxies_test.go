package desktop

import (
	"encoding/json"
	"os"
	"testing"
)

func TestProxySharedContractAndRealBridge(t *testing.T) {
	raw, e := os.ReadFile("../../desktop/contract/proxies.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		Contract   string         `json:"contract"`
		Workspace  ProxyWorkspace `json:"workspace"`
		Bind       ProxyAction    `json:"bind"`
		SystemBind ProxyAction    `json:"system_bind"`
	}
	if e = json.Unmarshal(raw, &f); e != nil || f.Contract != Contract || f.Bind.Binding.Mode != "proxy" || f.SystemBind.Binding.Mode != "system" {
		t.Fatal(e)
	}
	s := testSession(t)
	b := NewBridge(s)
	requireReady(t, b)
	w := b.ProxyWorkspace()
	if w.Problem != nil || w.Data == nil {
		t.Fatal(w)
	}
	secret := "http://user:bridge-secret@localhost:1234"
	in := ProxyAction{Action: "put", Profile: ProxyProfile{ID: "edge", Name: "Edge", Kind: "http"}, Secret: &secret, Approved: true, ExpectedRevision: w.Data.Revision}
	out := b.ManageProxy(in)
	if out.Problem != nil || out.Data == nil || len(out.Data.Profiles) != 1 {
		t.Fatal(out)
	}
	stale := b.ManageProxy(in)
	if stale.Problem == nil || stale.Problem.Code != "conflict" {
		t.Fatal(stale)
	}
	created := b.SaveProvider(SaveProvider{Provider: Provider{ID: "demo", BaseURLs: []string{"https://upstream.example/v1"}, Enabled: true, TimeoutSeconds: 120, MaxConcurrent: 1}})
	if created.Problem != nil {
		t.Fatal(created)
	}
	system := f.SystemBind
	system.ExpectedRevision = out.Data.Revision
	system.Binding.KeyID = ""
	result := b.ManageProxy(system)
	if result.Problem != nil || result.Data == nil {
		t.Fatal(result)
	}
	found := false
	for _, binding := range result.Data.Bindings {
		if binding.ProviderID == "demo" && binding.Mode == "system" {
			found = true
		}
	}
	if !found {
		t.Fatal("system binding not persisted")
	}
	in.Approved = false
	if x := b.ManageProxy(in); x.Problem == nil || x.Problem.Code != "invalid" {
		t.Fatal(x)
	}
}
