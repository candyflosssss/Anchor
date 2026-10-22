package managed

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/candyflosssss/Anchor/internal/state"
)

const desktopProfileFixture = "12345678-1234-4321-abcd-123456789abc"
const desktopOtherFixture = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func desktopFixture(t *testing.T, env *Environment) (string, string) {
	t.Helper()
	env.ClaudeDesktopHome = filepath.Join(env.Home, "Claude-3p")
	root := filepath.Join(env.ClaudeDesktopHome, "configLibrary")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(map[string]any{"appliedId": desktopProfileFixture, "entries": []any{map[string]any{"id": desktopProfileFixture, "name": "Local"}}})
	writeClaudeFixture(t, filepath.Join(root, "_meta.json"), meta)
	path := filepath.Join(root, desktopProfileFixture+".json")
	writeClaudeFixture(t, path, []byte(`{"inferenceProvider":"anthropic","inferenceGatewayApiKey":"previous-private","inferenceCredentialHelper":"/old/helper","inferenceCustomHeaders":{"tenant":"old"},"deploymentDisplayName":"keep","inferenceModels":["original-model"]}`))
	return root, path
}
func writeClaudeFixture(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeDesktopAndCLIIndependentLifecycle(t *testing.T) {
	m, env, _ := workspaceManager(t)
	root, path := desktopFixture(t, &env)
	if claudeDesktopPolicyPresent(root) {
		t.Skip("device managed by Desktop policy")
	}
	meta := readSyncFile(t, filepath.Join(root, "_meta.json"))
	original := readSyncFile(t, path)
	cliRoot := filepath.Join(env.Home, ".claude")
	if err := os.Mkdir(cliRoot, 0700); err != nil {
		t.Fatal(err)
	}
	cliPath := filepath.Join(cliRoot, "settings.json")
	writeClaudeFixture(t, cliPath, []byte(`{"hooks":{"Stop":[]}}`))
	if _, err := m.Store.PutModel(state.Model{ID: "中文别名", DisplayName: "日常编程", Candidates: []state.Candidate{{ProviderID: "p", Model: "actual"}}}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	targets, err := Discover(env)
	if err != nil {
		t.Fatal(err)
	}
	bindings := map[string]state.Binding{}
	for _, target := range targets {
		if target.Agent != "claude" && target.Agent != "claude-desktop" {
			continue
		}
		b, err := m.Enroll(env, EnrollAgent{TargetID: target.ID, Model: "中文别名", Approved: true})
		if err != nil {
			t.Fatal(target, err)
		}
		bindings[target.Agent] = b
	}
	if len(bindings) != 2 || bindings["claude-desktop"].ClaudeDesktopProfile != desktopProfileFixture {
		t.Fatal("missing independent bindings", bindings)
	}
	cliToken, _ := m.Store.BindingToken(bindings["claude"].ID)
	desktopToken, _ := m.Store.BindingToken(bindings["claude-desktop"].ID)
	if cliToken == "" || desktopToken == "" || cliToken == desktopToken {
		t.Fatal("credentials not scoped")
	}
	var cfg struct {
		Provider string                                 `json:"inferenceProvider"`
		Base     string                                 `json:"inferenceGatewayBaseUrl"`
		Key      string                                 `json:"inferenceGatewayApiKey"`
		Models   []struct{ Name, LabelOverride string } `json:"inferenceModels"`
	}
	if err := json.Unmarshal(readSyncFile(t, path), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "gateway" || cfg.Key != desktopToken || cfg.Base != "http://127.0.0.1:17860" || len(cfg.Models) != 2 || cfg.Models[0].Name != "中文别名" || cfg.Models[0].LabelOverride != "日常编程" {
		t.Fatal("desktop routing/default/list mismatch")
	}
	var cli struct {
		Picker struct {
			Options []struct{ Model, Label string }
			Replace bool `json:"replaceBuiltInOptions"`
		} `json:"modelPicker"`
	}
	if err := json.Unmarshal(readSyncFile(t, cliPath), &cli); err != nil || !cli.Picker.Replace || len(cli.Picker.Options) != 2 {
		t.Fatal("CLI aliases not listed", err)
	}
	if _, err := m.Store.PutModel(state.Model{ID: "third", Candidates: []state.Candidate{{ProviderID: "p", Model: "third-upstream"}}}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SyncMappings(); err != nil {
		t.Fatal(err)
	}
	for agent, b := range bindings {
		g, err := m.gateway(b)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := AdapterFor(agent).Verify(b, g); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Contains(readSyncFile(t, path), []byte("third")) || !bytes.Contains(readSyncFile(t, cliPath), []byte("third")) {
		t.Fatal("mapping sync missing")
	}
	if err := m.Store.DeleteModel("third", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SyncMappings(); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(readSyncFile(t, path), []byte("third")) || bytes.Contains(readSyncFile(t, cliPath), []byte("third")) {
		t.Fatal("deleted mapping remains in a client picker")
	}
	cliRaw := readSyncFile(t, cliPath)
	b := bindings["claude-desktop"]
	changed := b
	changed.ClaudeDesktopProfile = desktopOtherFixture
	if _, err := m.Store.UpdateBindingCatalog(changed, b.Revision); !errors.Is(err, state.ErrInvalid) {
		t.Fatal("profile identity changed", err)
	}
	if _, r, err := m.SetEnabled(b.ID, b.Revision, false); err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
	if !bytes.Equal(cliRaw, readSyncFile(t, cliPath)) || !bytes.Equal(meta, readSyncFile(t, filepath.Join(root, "_meta.json"))) {
		t.Fatal("Desktop modified CLI or selection")
	}
	var restored, expected any
	_ = json.Unmarshal(readSyncFile(t, path), &restored)
	_ = json.Unmarshal(original, &expected)
	if !reflect.DeepEqual(restored, expected) {
		t.Fatal("desktop baseline not restored")
	}
	if _, err := m.Store.BindingForToken(cliToken); err != nil {
		t.Fatal("CLI disabled with Desktop", err)
	}
	if _, err := m.Store.BindingForToken(desktopToken); !errors.Is(err, state.ErrNotFound) {
		t.Fatal("Desktop token still active")
	}
}

func TestClaudeDesktopSelectionAndUntrustedConfiguration(t *testing.T) {
	m, env, _ := workspaceManager(t)
	root, path := desktopFixture(t, &env)
	if claudeDesktopPolicyPresent(root) {
		t.Skip("device managed by Desktop policy")
	}
	b := state.Binding{ID: "desktop", AdapterID: "claude-desktop", AdapterVersion: "1", ConfigRoot: root, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true}
	if err := ValidateTarget(&b); err != nil {
		t.Fatal(err)
	}
	g := GatewayBinding{Ready: true, Token: "fixture-token", BaseURL: "http://127.0.0.1:17860/v1", Models: []state.Model{{ID: "public"}}}
	p, err := AdapterFor(b.AdapterID).Plan(b, g)
	if err != nil {
		t.Fatal(err)
	}
	preview, _ := json.Marshal(p)
	if bytes.Contains(preview, []byte(g.Token)) || bytes.Contains(preview, []byte("previous-private")) {
		t.Fatal("plan leaked secrets")
	}
	oldMeta := readSyncFile(t, filepath.Join(root, "_meta.json"))
	otherMeta := []byte(`{"appliedId":"` + desktopOtherFixture + `","entries":[{"id":"` + desktopOtherFixture + `"}]}`)
	otherPath := filepath.Join(root, desktopOtherFixture+".json")
	writeClaudeFixture(t, otherPath, []byte(`{"deploymentDisplayName":"other"}`))
	writeClaudeFixture(t, filepath.Join(root, "_meta.json"), otherMeta)
	before := readSyncFile(t, path)
	if _, err := m.Engine.Apply(p); !errors.Is(err, ErrChanged) || !bytes.Equal(before, readSyncFile(t, path)) {
		t.Fatal("selection changed after planning", err)
	}
	writeClaudeFixture(t, filepath.Join(root, "_meta.json"), oldMeta)
	if _, err := m.Engine.Apply(p); err != nil {
		t.Fatal(err)
	}
	writeClaudeFixture(t, filepath.Join(root, "_meta.json"), otherMeta)
	if _, err := AdapterFor(b.AdapterID).Verify(b, g); !errors.Is(err, ErrChanged) {
		t.Fatal("selection followed silently", err)
	}
	if r, err := m.Engine.Recover(b.ID); err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
	if !bytes.Contains(readSyncFile(t, otherPath), []byte("other")) || !bytes.Equal(otherMeta, readSyncFile(t, filepath.Join(root, "_meta.json"))) {
		t.Fatal("recovery changed selected profile")
	}
	for _, raw := range []string{
		`{"appliedId":"../../unsafe","entries":[]}`,
		`{"appliedId":"` + desktopProfileFixture + `","entries":[],"hybridPointer":{"source":"remote"}}`,
		`{"appliedId":"` + desktopProfileFixture + `","entries":[]}`,
	} {
		writeClaudeFixture(t, filepath.Join(root, "_meta.json"), []byte(raw))
		if _, err := inspectClaudeDesktop(b); err == nil {
			t.Fatal("unsafe metadata accepted")
		}
	}
	writeClaudeFixture(t, filepath.Join(root, "_meta.json"), oldMeta)
	writeClaudeFixture(t, path, []byte(`{"bootstrapUrl":"https://example.invalid/bootstrap"}`))
	if _, err := inspectClaudeDesktop(b); err == nil {
		t.Fatal("bootstrap configuration accepted")
	}
	if supportedConfig(filepath.Join(root, "_meta.json")) || supportedConfig(filepath.Join(env.Home, desktopProfileFixture+".json")) {
		t.Fatal("recovery path allowlist too broad")
	}
}

func TestClaudeDesktopPathsAndSetupDiscovery(t *testing.T) {
	for _, goos := range []string{"linux", "windows", "darwin"} {
		path := claudeDesktopDataRoot("/home/fixture", "", "", goos)
		if !filepath.IsAbs(path) || filepath.Base(path) != "Claude-3p" {
			t.Fatal(goos, path)
		}
	}
	env := Environment{Home: t.TempDir()}
	env.ClaudeDesktopHome = filepath.Join(env.Home, "Claude-3p")
	if err := os.Mkdir(env.ClaudeDesktopHome, 0700); err != nil {
		t.Fatal(err)
	}
	targets, err := Discover(env)
	if err != nil || len(targets) != 1 || targets[0].Status != "desktop_setup_required" {
		t.Fatal(targets, err)
	}
}

func TestClaudeCLILegacyJournalAddsPickerAndRestoresBaseline(t *testing.T) {
	m, env, _ := workspaceManager(t)
	root := filepath.Join(env.Home, ".claude")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "settings.json")
	original := []byte(`{"modelPicker":{"options":[{"model":"before","label":"Original"}]},"env":{"CLAUDE_CODE_USE_ANTHROPIC_AWS":"1"},"hooks":{"Stop":[]}}`)
	writeClaudeFixture(t, path, original)
	b, err := m.Add(state.Binding{ID: "legacy-claude", AdapterID: "claude", AdapterVersion: "1", ConfigRoot: root, Executable: env.Executable, Model: "public", ModelScope: "all-mapped", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	g, err := m.gateway(b)
	if err != nil {
		t.Fatal(err)
	}
	g.Ready = true
	p, err := AdapterFor("claude").Plan(b, g)
	if err != nil {
		t.Fatal(err)
	}
	// Produce the previous adapter's journal: these fields were not owned yet.
	p.Edits = p.Edits[:len(p.Edits)-2]
	p.Fields = p.Fields[:len(p.Fields)-2]
	if _, err := m.Engine.Apply(p); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	if _, err := AdapterFor("claude").Verify(b, g); err != nil {
		t.Fatal("legacy binding was not migrated", err)
	}
	if _, r, err := m.SetEnabled(b.ID, b.Revision, false); err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
	var expected, actual any
	_ = json.Unmarshal(original, &expected)
	_ = json.Unmarshal(readSyncFile(t, path), &actual)
	if !reflect.DeepEqual(expected, actual) {
		t.Fatal("new fields lost their pre-migration baseline")
	}
}

func TestClaudeDesktopRejectsUnsafeProfileFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "duplicate", "invalid"} {
		t.Run(kind, func(t *testing.T) {
			_, env, _ := workspaceManager(t)
			root, path := desktopFixture(t, &env)
			if claudeDesktopPolicyPresent(root) {
				t.Skip("device managed by Desktop policy")
			}
			switch kind {
			case "symlink", "hardlink":
				other := filepath.Join(env.Home, "unrelated.json")
				writeClaudeFixture(t, other, []byte(`{"keep":true}`))
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				link := os.Link
				if kind == "symlink" {
					link = os.Symlink
				}
				if err := link(other, path); err != nil {
					t.Skip(err)
				}
			case "duplicate":
				writeClaudeFixture(t, path, []byte(`{"inferenceProvider":"gateway","inferenceProvider":"anthropic"}`))
			case "invalid":
				writeClaudeFixture(t, path, []byte(`{"broken":`))
			}
			if _, err := inspectClaudeDesktop(state.Binding{ConfigRoot: root}); err == nil {
				t.Fatal("unsafe profile accepted")
			}
		})
	}
}
