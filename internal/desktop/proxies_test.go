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
		Contract  string         `json:"contract"`
		Workspace ProxyWorkspace `json:"workspace"`
		Bind      ProxyAction    `json:"bind"`
	}
	if e = json.Unmarshal(raw, &f); e != nil || f.Contract != Contract || f.Bind.Binding.Mode != "proxy" {
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
	in.Approved = false
	if x := b.ManageProxy(in); x.Problem == nil || x.Problem.Code != "invalid" {
		t.Fatal(x)
	}
}
