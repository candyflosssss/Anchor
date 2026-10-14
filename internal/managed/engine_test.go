package managed

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/candyflosssss/Anchor/internal/state"
)

func target(t *testing.T, original string) (*state.Store, state.Binding, GatewayBinding, *Engine) {
	t.Helper()
	st, e := state.Open(t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { st.Close() })
	if _, e = st.Initialize("127.0.0.1:17861", "127.0.0.1:17860"); e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	if e = os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	if original != "" {
		if e = os.WriteFile(filepath.Join(root, "config.toml"), []byte(original), 0640); e != nil {
			t.Fatal(e)
		}
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	b := state.Binding{ID: "test", AdapterID: "codex", AdapterVersion: "1", ConfigRoot: root, Model: "public", Executable: exe, Enabled: true}
	return st, b, GatewayBinding{BaseURL: "http://127.0.0.1:17860/v1", Model: b.Model, DataDir: st.Dir, Ready: true, Tools: true, Streaming: true}, NewEngine(st)
}
func planApply(t *testing.T, b state.Binding, g GatewayBinding, e *Engine) Plan {
	t.Helper()
	p, err := (Codex{}).Plan(b, g)
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Apply(p)
	if err != nil || r.Status != "applied" {
		t.Fatal(r, err)
	}
	return p
}
func TestApplyRecoverPreservesUnownedConfiguration(t *testing.T) {
	original := "# user comment\nmodel = \"old\"\nmodel_provider = \"original\"\nsecret = \"PRIVATE-FIXTURE\"\n[projects.\"/example\"]\ntrust_level = \"trusted\"\n"
	_, b, g, e := target(t, original)
	p := planApply(t, b, g, e)
	preview, _ := json.Marshal(p)
	if bytes.Contains(preview, []byte("PRIVATE-FIXTURE")) || bytes.Contains(preview, []byte("original")) {
		t.Fatal("plan leaked private values", string(preview))
	}
	raw, _ := os.ReadFile(p.Path)
	if !bytes.Contains(raw, []byte("# user comment")) || !bytes.Contains(raw, []byte(`secret = "PRIVATE-FIXTURE"`)) {
		t.Fatal(string(raw))
	}
	var decoded struct {
		ModelProviders map[string]struct {
			Auth struct {
				Timeout uint64 `toml:"timeout_ms"`
			} `toml:"auth"`
		} `toml:"model_providers"`
	}
	if _, err := toml.Decode(string(raw), &decoded); err != nil || decoded.ModelProviders["anchor_test"].Auth.Timeout != 5000 {
		t.Fatal("integer auth configuration invalid", err, string(raw))
	}
	raw = append(raw, []byte("\n# editor addition\n[extra]\nvalue = 7\n")...)
	if err := os.WriteFile(p.Path, raw, 0640); err != nil {
		t.Fatal(err)
	}
	r, err := e.Recover(b.ID)
	if err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
	now, _ := os.ReadFile(p.Path)
	m, err := document(now)
	if err != nil || m["model"] != "old" || m["model_provider"] != "original" || m["web_search"] != nil || m["extra"] == nil || bytes.Contains(now, []byte("anchor_test")) {
		t.Fatal(string(now), err)
	}
	info, _ := os.Stat(p.Path)
	if info.Mode().Perm() != 0640 {
		t.Fatal("permissions changed", info.Mode())
	}
	r, err = e.Recover(b.ID)
	if err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
}
func TestRecoveryThirdValueAndMissingFile(t *testing.T) {
	for _, kind := range []string{"third", "removed", "already_original"} {
		t.Run(kind, func(t *testing.T) {
			_, b, g, e := target(t, "model = \"old\"\n")
			p := planApply(t, b, g, e)
			raw, _ := os.ReadFile(p.Path)
			switch kind {
			case "third":
				raw = []byte(strings.Replace(string(raw), `model = "public"`, `model = "user-choice"`, 1))
				_ = os.WriteFile(p.Path, raw, 0600)
			case "removed":
				_ = os.Remove(p.Path)
			case "already_original":
				_ = os.WriteFile(p.Path, []byte("model = \"old\"\n"), 0600)
			}
			r, err := e.Recover(b.ID)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "already_original" {
				if r.Status != "restored" {
					t.Fatal(r)
				}
			} else {
				if r.Status != "conflict" {
					t.Fatal(r)
				}
			}
			now, readErr := os.ReadFile(p.Path)
			if kind == "third" && !bytes.Contains(now, []byte("user-choice")) {
				t.Fatal(string(now))
			}
			if kind == "removed" && !errors.Is(readErr, os.ErrNotExist) {
				t.Fatal("removed file recreated")
			}
		})
	}
}
func TestArtifactReferencePreserved(t *testing.T) {
	_, b, g, e := target(t, "model = \"old\"\n")
	p := planApply(t, b, g, e)
	f, err := os.OpenFile(p.Path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("\n[profiles.user]\nmodel_provider = \"anchor_test\"\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Recover(b.ID)
	if err != nil || r.Status != "conflict" {
		t.Fatal(r, err)
	}
	now, _ := os.ReadFile(p.Path)
	m, _ := document(now)
	if m["model"] != "old" || !at(m, []string{"model_providers", "anchor_test"}).Exists {
		t.Fatal(string(now))
	}
}
func TestStalePlanAndCrashRecovery(t *testing.T) {
	for _, written := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_publish", true: "after_publish"}[written], func(t *testing.T) {
			_, b, g, e := target(t, "model = \"old\"\n")
			p, err := (Codex{}).Plan(b, g)
			if err != nil {
				t.Fatal(err)
			}
			if err = e.prepare(); err != nil {
				t.Fatal(err)
			}
			j := journal{Schema: 1, BindingID: b.ID, AdapterVersion: "1", OperationID: "crash-fixture", Path: p.Path, Original: p.original, Existed: true, Edits: p.Edits, Status: "prepared"}
			if err = e.save(j); err != nil {
				t.Fatal(err)
			}
			if written {
				next, err := e.Editor.Patch(p.original, p.Edits)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(p.Path, next, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = e.Apply(p); err == nil {
				t.Fatal("active recovery record overwritten")
			}
			r, err := e.Recover(b.ID)
			if err != nil || r.Status != "restored" {
				t.Fatal(r, err)
			}
			now, _ := os.ReadFile(p.Path)
			m, _ := document(now)
			if m["model"] != "old" {
				t.Fatal(string(now))
			}
		})
	}
	_, b, g, e := target(t, "model = \"old\"\n")
	p, err := (Codex{}).Plan(b, g)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(p.Path, []byte("model = \"new\"\n"), 0600)
	if _, err = e.Apply(p); !errors.Is(err, ErrChanged) {
		t.Fatal(err)
	}
	if _, err = os.Stat(e.journalPath(b.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("stale plan created journal")
	}
}
func TestUnsafeTOMLAndTargets(t *testing.T) {
	for _, raw := range []string{"model = \"\"\"old\"\"\"\n", "\"model\" = \"old\"\n", "profile = \"custom\"\n", "[features]\nresponses_websockets = true\n", "model = \"old\"\nmodel = \"dup\"\n", "model_providers.anchor_test = { name = \"user\" }\n"} {
		t.Run(raw, func(t *testing.T) {
			_, b, g, _ := target(t, raw)
			if _, err := (Codex{}).Plan(b, g); err == nil {
				t.Fatal("unsafe config accepted", raw)
			}
		})
	}
	_, b, g, _ := target(t, "")
	elsewhere := filepath.Join(t.TempDir(), "config.toml")
	_ = os.WriteFile(elsewhere, []byte("model = \"safe\"\n"), 0600)
	if err := os.Symlink(elsewhere, filepath.Join(b.ConfigRoot, "config.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Codex{}).Plan(b, g); err == nil {
		t.Fatal("linked config accepted")
	}
}
func TestRepeatedLifecycleDoesNotRebaseOriginal(t *testing.T) {
	st, b, _, _ := target(t, "model = \"old\"\n")
	if _, err := st.PutProvider(state.Provider{ID: "p", BaseURL: "http://127.0.0.1:8080/v1", Protocol: "responses", Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutModel(state.Model{ID: b.Model, Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddBinding(b); err != nil {
		t.Fatal(err)
	}
	m := NewManager(st)
	for i := 0; i < 2; i++ {
		r, err := m.Start()
		if err != nil || len(r) != 1 || r[0].Status != "applied" {
			t.Fatal(r, err)
		}
	}
	r, err := m.Stop()
	if err != nil || r[0].Status != "restored" {
		t.Fatal(r, err)
	}
	raw, _ := os.ReadFile(filepath.Join(b.ConfigRoot, "config.toml"))
	doc, _ := document(raw)
	if doc["model"] != "old" {
		t.Fatal(string(raw))
	}
}

func TestStartupPreparedAndPartialFailure(t *testing.T) {
	for _, written := range []bool{false, true} {
		t.Run(map[bool]string{false: "unpublished", true: "published"}[written], func(t *testing.T) {
			st, b, g, e := target(t, "model = \"old\"\n")
			p, err := (Codex{}).Plan(b, g)
			if err != nil {
				t.Fatal(err)
			}
			if err = e.prepare(); err != nil {
				t.Fatal(err)
			}
			j := journal{Schema: 1, BindingID: b.ID, AdapterVersion: "1", OperationID: "prepared", Path: p.Path, Original: p.original, Existed: true, Edits: p.Edits, Status: "prepared"}
			if err = e.save(j); err != nil {
				t.Fatal(err)
			}
			if written {
				next, err := e.Editor.Patch(p.original, p.Edits)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(p.Path, next, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := st.PutProvider(state.Provider{ID: "p", BaseURL: "http://127.0.0.1:8080/v1", Protocol: "responses", Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, 0, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := st.PutModel(state.Model{ID: b.Model, Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}, 0); err != nil {
				t.Fatal(err)
			}
			if _, err = st.AddBinding(b); err != nil {
				t.Fatal(err)
			}
			broken := b
			broken.ID = "broken"
			broken.ConfigRoot = t.TempDir()
			_ = os.Chmod(broken.ConfigRoot, 0700)
			if _, err = st.AddBinding(broken); err != nil {
				t.Fatal(err)
			}
			_ = os.WriteFile(filepath.Join(broken.ConfigRoot, "config.toml"), []byte("profile = \"external\"\n"), 0600)
			m := NewManager(st)
			results, err := m.Start()
			if err != nil {
				t.Fatal(err)
			}
			statuses := map[string]string{}
			for _, r := range results {
				statuses[r.BindingID] = r.Status
			}
			if statuses[b.ID] != "applied" || statuses[broken.ID] != "apply_failed" {
				t.Fatal(statuses)
			}
			statuses = map[string]string{}
			results, err = m.Status()
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range results {
				statuses[r.BindingID] = r.Status
			}
			if statuses[broken.ID] != "apply_failed" {
				t.Fatal("startup failure hidden", statuses)
			}
			if _, err = m.Stop(); err != nil {
				t.Fatal(err)
			}
			raw, _ := os.ReadFile(p.Path)
			doc, _ := document(raw)
			if doc["model"] != "old" {
				t.Fatal(string(raw))
			}
		})
	}
}

func TestRejectWrongTypedManagedFieldAndHardLink(t *testing.T) {
	for _, raw := range []string{"model = 1\n", "web_search = false\n", "model_provider = [\"x\"]\n"} {
		_, b, g, _ := target(t, raw)
		if _, err := (Codex{}).Plan(b, g); err == nil {
			t.Fatal("non-reversible original accepted", raw)
		}
	}
	_, b, g, _ := target(t, "model = \"old\"\n")
	source := filepath.Join(b.ConfigRoot, "config.toml")
	other := filepath.Join(t.TempDir(), "copy")
	if err := os.Link(source, other); err != nil {
		t.Fatal(err)
	}
	if _, err := (Codex{}).Plan(b, g); err == nil {
		t.Fatal("hardlinked config accepted")
	}
}

func TestStartupErrorsNeverReportApplied(t *testing.T) {
	for _, kind := range []string{"invalid_config", "unsupported_profile"} {
		t.Run(kind, func(t *testing.T) {
			st, b, g, e := target(t, "model = \"old\"\n")
			if _, err := st.PutProvider(state.Provider{ID: "p", BaseURL: "http://127.0.0.1:8080/v1", Protocol: "responses", Enabled: true, TimeoutSeconds: 5, MaxConcurrent: 1}, 0, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := st.PutModel(state.Model{ID: b.Model, Tools: true, Streaming: true, Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}, 0); err != nil {
				t.Fatal(err)
			}
			var err error
			b, err = st.AddBinding(b)
			if err != nil {
				t.Fatal(err)
			}
			p := planApply(t, b, g, e)
			raw, err := os.ReadFile(p.Path)
			if err != nil {
				t.Fatal(err)
			}
			addition := "profile = \"external\"\n"
			want := "apply_failed"
			if kind == "invalid_config" {
				addition = "invalid TOML\n"
				want = "recovery_pending"
			}
			if err = os.WriteFile(p.Path, append([]byte(addition), raw...), 0600); err != nil {
				t.Fatal(err)
			}
			m := NewManager(st)
			for _, check := range []func() ([]Result, error){m.Start, m.Status} {
				results, err := check()
				if err != nil || len(results) != 1 || results[0].Status != want {
					t.Fatal(results, err, "want", want)
				}
			}
			// Removing the external change allows an explicit apply to clear
			// the retained startup failure, without capturing a new original.
			if err = os.WriteFile(p.Path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = m.Apply(b.ID, b.Revision, ""); err != nil {
				t.Fatal(err)
			}
			results, err := m.Status()
			if err != nil || len(results) != 1 || results[0].Status != "applied" {
				t.Fatal(results, err)
			}
			if _, err = m.Stop(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestChineseBindingApplyAndRecover(t *testing.T) {
	_, b, g, e := target(t, "model = \"old\"\n")
	b.ID = "中文助手"
	b.Model = "中文模型"
	g.Model = b.Model
	p := planApply(t, b, g, e)
	raw, err := os.ReadFile(p.Path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := document(raw)
	if err != nil || doc["model"] != b.Model || doc["model_provider"] != "anchor_"+b.ID {
		t.Fatal(doc, err)
	}
	if _, err := os.Stat(e.journalPath(b.ID)); err != nil {
		t.Fatal(err)
	}
	result, err := e.Recover(b.ID)
	if err != nil || result.Status != "restored" {
		t.Fatal(result, err)
	}
	raw, err = os.ReadFile(p.Path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err = document(raw)
	if err != nil || doc["model"] != "old" {
		t.Fatal(doc, err)
	}
}
