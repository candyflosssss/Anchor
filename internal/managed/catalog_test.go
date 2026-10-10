package managed

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

func metadata(t *testing.T) *state.CodexMetadata {
	t.Helper()
	raw, err := os.ReadFile("../state/testdata/codex-models.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := state.ImportCodexMetadata(raw, "fixture-source", "0.159.3")
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func catalogTarget(t *testing.T, original string) (*state.Store, state.Binding, GatewayBinding, *Engine) {
	t.Helper()
	s, b, g, e := target(t, original)
	b.ModelScope = "all-compatible"
	b.CodexVersion = "0.159.3"
	g.CatalogVersion = b.CodexVersion
	g.Models = []state.Model{{ID: b.Model, DisplayName: "Public alias", Tools: true, Streaming: true, Codex: metadata(t)},
		{ID: "alternative", Tools: true, Streaming: true, Codex: metadata(t)}}
	return s, b, g, e
}
func TestCatalogCopiesMetadataAndRedactsPlan(t *testing.T) {
	_, b, g, e := catalogTarget(t, "model = \"old\"\nmodel_catalog_json = \"/old/user.json\"\nsecret = \"PRIVATE\"\n")
	p := planApply(t, b, g, e)
	if len(p.AvailableModels) != 2 || p.CatalogVersion != "0.159.3" {
		t.Fatal(p)
	}
	raw, err := os.ReadFile(p.CatalogPath)
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Models []map[string]any `json:"models"`
	}
	if err = json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	for i, m := range catalog.Models {
		if m["slug"] != g.Models[i].ID || m["upgrade"] != nil || m["prefer_websockets"] != false || m["use_responses_lite"] != false || m["visibility"] != "list" || m["supported_in_api"] != true {
			t.Fatal(m)
		}
		if m["context_window"] != float64(32000) || !strings.Contains(m["base_instructions"].(string), "Test fixture") {
			t.Fatal("metadata rewritten")
		}
	}
	preview, _ := json.Marshal(p)
	if bytes.Contains(preview, []byte("PRIVATE")) || bytes.Contains(preview, []byte("Test fixture")) {
		t.Fatal("plan contains private data/instructions")
	}
	if _, err = (Codex{}).Verify(b, g); err != nil {
		t.Fatal(err)
	}
	r, err := e.Recover(b.ID)
	if err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
	docRaw, _ := os.ReadFile(p.Path)
	doc, _ := document(docRaw)
	if doc["model_catalog_json"] != "/old/user.json" || doc["secret"] != "PRIVATE" {
		t.Fatal(doc)
	}
	if _, err = os.Stat(p.CatalogPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("catalog not removed", err)
	}
	if _, err = (Codex{}).Plan(b, g); err != nil {
		t.Fatal("cannot plan again", err)
	}
}
func TestCatalogRecoveryConflictsAndReferences(t *testing.T) {
	for _, mode := range []string{"edited", "deleted", "profile-absolute", "profile-relative", "unsafe-profile", "base-reference"} {
		t.Run(mode, func(t *testing.T) {
			_, b, g, e := catalogTarget(t, "model = \"old\"\n")
			p := planApply(t, b, g, e)
			switch mode {
			case "edited":
				os.WriteFile(p.CatalogPath, []byte(`{"user":"keep"}`), 0600)
			case "deleted":
				os.Remove(p.CatalogPath)
			case "profile-absolute", "profile-relative":
				path := p.CatalogPath
				if mode == "profile-relative" {
					path = "./" + filepath.Base(path)
				}
				escaped, _ := json.Marshal(path)
				os.WriteFile(filepath.Join(b.ConfigRoot, "work.config.toml"), []byte("model_provider = \"anchor_test\"\nmodel_catalog_json = "+string(escaped)+"\n"), 0600)
			case "unsafe-profile":
				if err := os.Symlink(p.Path, filepath.Join(b.ConfigRoot, "work.config.toml")); err != nil {
					t.Skip(err)
				}
			case "base-reference":
				raw, _ := os.ReadFile(p.Path)
				// A user-authored table retains the generated file independently.
				escaped, _ := json.Marshal(p.CatalogPath)
				os.WriteFile(p.Path, append(raw, []byte("\n[user]\nmodel_catalog_json = "+string(escaped)+"\n")...), 0600)
			}
			if mode == "edited" || mode == "deleted" {
				if r, err := e.Status(b.ID); err != nil || r.Status != "overridden" {
					t.Fatal(r, err)
				}
				if _, err := (Codex{}).Verify(b, g); !errors.Is(err, ErrChanged) {
					t.Fatal(err)
				}
			}
			r, err := e.Recover(b.ID)
			if mode == "unsafe-profile" {
				if err == nil {
					t.Fatal("unsafe reference inspected")
				}
			} else if err != nil || r.Status != map[bool]string{true: "restored", false: "conflict"}[mode == "deleted"] {
				t.Fatal(r, err)
			}
			if mode != "deleted" {
				raw, err := os.ReadFile(p.CatalogPath)
				if err != nil {
					t.Fatal("referenced/user artifact removed", err)
				}
				if mode == "edited" && string(raw) != `{"user":"keep"}` {
					t.Fatal(string(raw))
				}
			}
		})
	}
}
func TestCatalogCollisionAndPlanFingerprint(t *testing.T) {
	_, b, g, e := catalogTarget(t, "")
	p, err := (Codex{}).Plan(b, g)
	if err != nil {
		t.Fatal(err)
	}
	g.Models[1].DisplayName = "Changed name"
	changed, err := (Codex{}).Plan(b, g)
	if err != nil || changed.Fingerprint == p.Fingerprint {
		t.Fatal(changed, err)
	}
	if err = os.WriteFile(p.CatalogPath, []byte("user-owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = (Codex{}).Plan(b, g); err == nil {
		t.Fatal("reserved collision accepted")
	}
	if _, err = e.Apply(p); !errors.Is(err, ErrChanged) {
		t.Fatal("stale catalog absence accepted", err)
	}
	if raw, _ := os.ReadFile(p.CatalogPath); string(raw) != "user-owned" {
		t.Fatal("overwrote user file")
	}
	os.Remove(p.CatalogPath)
	other := filepath.Join(b.ConfigRoot, "other.json")
	os.WriteFile(other, []byte("user-owned"), 0600)
	if err = os.Link(other, p.CatalogPath); err == nil {
		if _, err = (Codex{}).Plan(b, g); err == nil {
			t.Fatal("hardlink catalog accepted")
		}
		os.Remove(p.CatalogPath)
	}
	if err = os.Symlink(other, p.CatalogPath); err == nil {
		if _, err = (Codex{}).Plan(b, g); err == nil {
			t.Fatal("symlink catalog accepted")
		}
	}
}
func TestCatalogCrashRecoveryEveryPublicationBoundary(t *testing.T) {
	for _, phase := range []string{"journal-only", "catalog-only", "both-files", "applied"} {
		t.Run(phase, func(t *testing.T) {
			_, b, g, e := catalogTarget(t, "model = \"old\"\n")
			p, err := (Codex{}).Plan(b, g)
			if err != nil {
				t.Fatal(err)
			}
			if err = e.prepare(); err != nil {
				t.Fatal(err)
			}
			j := journal{Schema: 1, BindingID: b.ID, AdapterVersion: "1", OperationID: "fixture", Path: p.Path, Original: p.original, Existed: p.existed, Edits: p.Edits, Catalog: p.catalog, Status: "prepared"}
			if phase == "applied" {
				j.Status = "applied"
			}
			if err = e.save(j); err != nil {
				t.Fatal(err)
			}
			if phase != "journal-only" {
				if err = os.WriteFile(p.CatalogPath, p.catalog.Written, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "both-files" || phase == "applied" {
				raw, err := e.Editor.Patch(p.original, p.Edits)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(p.Path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			r, err := e.Recover(b.ID)
			if err != nil || r.Status != "restored" {
				t.Fatal(r, err)
			}
			raw, _ := os.ReadFile(p.Path)
			doc, _ := document(raw)
			if doc["model"] != "old" || doc["model_catalog_json"] != nil {
				t.Fatal(doc)
			}
			if _, err = os.Stat(p.CatalogPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("orphan catalog", err)
			}
		})
	}
}
func TestManagerSetModelsMigrationAndConflicts(t *testing.T) {
	st, b, _, _ := catalogTarget(t, "model = \"old\"\n")
	if _, err := st.PutProvider(state.Provider{ID: "p", BaseURL: "http://127.0.0.1:1234/v1", Protocol: "responses", Enabled: true, TimeoutSeconds: 10, MaxConcurrent: 1}, 0, nil); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{b.Model, "alternative"} {
		if _, err := st.PutModel(state.Model{ID: id, Tools: true, Streaming: true, Codex: metadata(t), Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}, 0); err != nil {
			t.Fatal(err)
		}
	}
	b.ModelScope = ""
	b.CodexVersion = ""
	manager := NewManager(st)
	saved, err := manager.Add(b)
	if err != nil {
		t.Fatal(err)
	}
	token, _ := st.BindingToken(b.ID)
	if _, err = manager.Start(); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"", "invalid"} {
		if _, _, err = manager.SetModels(b.ID, 1, b.Model, scope); err == nil {
			t.Fatal("invalid explicit scope", scope)
		}
	}
	if _, _, err = manager.SetModels(b.ID, 2, b.Model, "all-compatible"); !errors.Is(err, state.ErrConflict) {
		t.Fatal(err)
	}
	saved, r, err := manager.SetModels(b.ID, 1, "alternative", "all-compatible")
	if err != nil || saved.Revision != 2 || r.Status != "restored" || saved.CodexVersion != "0.159.3" {
		t.Fatal(saved, r, err)
	}
	if now, _ := st.BindingToken(b.ID); now != token {
		t.Fatal("credential rotated")
	}
	p, err := manager.Plan(b.ID)
	if err != nil || len(p.AvailableModels) != 2 {
		t.Fatal(p, err)
	}
	stale := p.Fingerprint
	model, _ := st.Model(b.Model)
	model.DisplayName = "new name"
	if _, err = st.PutModel(model, model.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Apply(b.ID, 2, stale); !errors.Is(err, ErrChanged) {
		t.Fatal("stale metadata plan applied", err)
	}
	p, err = manager.Plan(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Apply(b.ID, 2, p.Fingerprint); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(p.CatalogPath, []byte(`{"user":"keep"}`), 0600)
	if _, _, err = manager.SetModels(b.ID, 2, b.Model, "single"); !errors.Is(err, ErrChanged) {
		t.Fatal("restore conflict broadened/changed policy", err)
	}
	unchanged, _ := st.Binding(b.ID)
	if unchanged.ModelScope != "all-compatible" || unchanged.Revision != 2 || unchanged.Model != "alternative" {
		t.Fatal(unchanged)
	}
}
