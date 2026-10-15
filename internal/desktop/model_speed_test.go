package desktop

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSpeedContractOptionalFields(t *testing.T) {
	out := TypeScript()
	for _, field := range []string{"speed_variants?: string[];", "service_tier?: string | null;", "service_tier?: string;"} {
		if !strings.Contains(out, field) {
			t.Fatal(field)
		}
	}
	if strings.Contains(out, ",omitempty") {
		t.Fatal("json options became a TypeScript property name")
	}
}

func TestSpeedVariantsBridgeReadWrite(t *testing.T) {
	b := NewBridge(testSession(t))
	requireReady(t, b)
	p := b.SaveProvider(SaveProvider{Provider: Provider{ID: "p", BaseURLs: []string{"http://127.0.0.1:1/v1"}, Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 1}})
	if p.Problem != nil || p.Data == nil {
		t.Fatal(p)
	}
	raw, err := os.ReadFile("../../desktop/contract/model-speed-variants.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Action   Action         `json:"action"`
		Variants []ModelMapping `json:"variants"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	written := b.ProviderAction(fixture.Action)
	if written.Problem != nil || written.Data == nil || len(written.Data.Models) != 3 {
		t.Fatal(written)
	}
	read := b.ProviderWorkspace(WorkspaceQuery{ProviderID: "p"})
	if read.Problem != nil || read.Data == nil {
		t.Fatal(read)
	}
	for _, want := range fixture.Variants {
		found := false
		for _, got := range read.Data.Models {
			if got.ID == want.ID {
				found = true
				if got.ServiceTier != want.ServiceTier || got.Upstream != want.Upstream {
					t.Fatal(got, want)
				}
			}
		}
		if !found {
			t.Fatal("missing variant", want.ID)
		}
	}
	in := fixture.Action
	in.ModelRevision = 1
	in.Model = "must-not-save"
	conflict := b.ProviderAction(in)
	if conflict.Problem == nil || conflict.Problem.Code != "conflict" {
		t.Fatal(conflict)
	}
	after := b.ProviderWorkspace(WorkspaceQuery{ProviderID: "p"})
	if after.Problem != nil || after.Data == nil {
		t.Fatal(after)
	}
	for _, got := range after.Data.Models {
		if got.Revision != 1 || got.Upstream != fixture.Action.Model {
			t.Fatal("partial write", got)
		}
	}
}
