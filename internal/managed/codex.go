package managed

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/candyflosssss/Anchor/internal/state"
)

type GatewayBinding struct {
	CodexRetries     *int
	BaseURL          string
	Model            string
	DataDir          string
	Ready            bool
	Tools            bool
	Streaming        bool
	Models           []state.Model
	CatalogVersion   string
	Token            string
	CatalogTemplates map[string]json.RawMessage
}
type Inspection struct {
	AdapterID           string   `json:"adapter_id"`
	AdapterVersion      string   `json:"adapter_version"`
	ConfigPath          string   `json:"config_path"`
	Exists              bool     `json:"exists"`
	Support             string   `json:"support"`
	EffectiveConnection string   `json:"effective_connection"`
	Reload              string   `json:"reload_policy"`
	Limitations         []string `json:"limitations"`
}

// Adapters only describe facts and field edits. The engine is the sole writer.
type Adapter interface {
	Discover(state.Binding) (Inspection, error)
	Inspect(state.Binding) (Inspection, error)
	Plan(state.Binding, GatewayBinding) (Plan, error)
	Verify(state.Binding, GatewayBinding) (Inspection, error)
}
type Codex struct{}

func (Codex) Discover(b state.Binding) (Inspection, error) { return (Codex{}).Inspect(b) }
func (Codex) Inspect(b state.Binding) (Inspection, error) {
	path := filepath.Join(b.ConfigRoot, "config.toml")
	raw, exists, e := readConfig(b.ConfigRoot, path)
	if e != nil {
		return Inspection{}, e
	}
	return inspectCodex(path, raw, exists)
}

func inspectCodex(path string, raw []byte, exists bool) (Inspection, error) {
	m, e := document(raw)
	if e != nil {
		return Inspection{}, e
	}
	if profile, ok := m["profile"]; ok && profile != "" {
		return Inspection{}, errors.New("selected Codex profiles are not supported by this adapter revision")
	}
	if features, ok := m["features"].(map[string]any); ok {
		for _, k := range []string{"responses_websockets", "responses_websockets_v2"} {
			if features[k] == true {
				return Inspection{}, errors.New("WebSocket transport is not supported; disable it before approval")
			}
		}
	}
	return Inspection{AdapterID: "codex", AdapterVersion: "1", ConfigPath: path, Exists: exists, Support: "configuration_only", EffectiveConnection: "unknown", Reload: "restart_required", Limitations: []string{"global config only; no selected profile or project scope", "CLI/environment/managed overrides and installed Codex version are not probed", "catalog version/upstream capabilities are operator-declared; restart remote app-server after changes", "no WebSocket, compaction endpoint, server state or real upstream verification"}}, nil
}
func (Codex) Plan(b state.Binding, g GatewayBinding) (Plan, error) {
	if e := b.Validate(); e != nil {
		return Plan{}, e
	}
	if !g.Ready || (b.ModelScope != "all-mapped" && (!g.Tools || !g.Streaming)) {
		return Plan{}, errors.New("gateway must be ready and selected model must advertise tools and streaming")
	}
	if _, e := (Codex{}).Inspect(b); e != nil {
		return Plan{}, e
	}
	path := filepath.Join(b.ConfigRoot, "config.toml")
	raw, exists, e := readConfig(b.ConfigRoot, path)
	if e != nil {
		return Plan{}, e
	}
	return (Codex{}).planFrom(b, g, raw, exists, "")
}

// planFrom also accepts a virtual restoration during an in-place refresh.
func (Codex) planFrom(b state.Binding, g GatewayBinding, raw []byte, exists bool, ownedCatalog string) (Plan, error) {
	if err := b.Validate(); err != nil {
		return Plan{}, err
	}
	if !g.Ready || (b.ModelScope != "all-mapped" && (!g.Tools || !g.Streaming)) {
		return Plan{}, ErrUnsafe
	}
	path := filepath.Join(b.ConfigRoot, "config.toml")
	if _, err := inspectCodex(path, raw, exists); err != nil {
		return Plan{}, err
	}
	provider := "anchor_" + strings.NewReplacer("-", "_", ".", "_").Replace(b.ID)
	paths := [][]string{{"model"}, {"model_provider"}, {"web_search"}, {"model_providers", provider}}
	var catalog []byte
	var modelIDs []string
	var catalogPath string
	var e error
	if len(g.Models) > 0 && (b.ModelScope == "all-mapped" || g.CatalogVersion != "") {
		catalog, modelIDs, e = buildCatalogTemplates(g.Models, g.CatalogVersion, g.CatalogTemplates)
		if e != nil {
			return Plan{}, e
		}
		catalogPath = filepath.Join(b.ConfigRoot, catalogName(b.ID))
		_, catalogExists, err := readConfig(b.ConfigRoot, catalogPath)
		if err != nil {
			return Plan{}, err
		}
		if catalogExists && catalogPath != ownedCatalog {
			return Plan{}, errors.New("reserved Anchor catalog already exists; recover or resolve conflict first")
		}
		paths = append(paths, []string{"model_catalog_json"})
	} else if b.ModelScope == "all-compatible" {
		return Plan{}, errors.New("all-compatible requires catalog metadata")
	}
	editor := TOMLEditor{}
	values, e := editor.Values(raw, paths)
	if e != nil {
		return Plan{}, e
	}
	for _, value := range values[:3] {
		if value.Exists {
			if _, ok := value.Data.(string); !ok {
				return Plan{}, ErrUnsafe
			}
		}
	}
	if len(values) > 4 && values[4].Exists {
		if _, ok := values[4].Data.(string); !ok {
			return Plan{}, ErrUnsafe
		}
	}
	if values[3].Exists {
		return Plan{}, errors.New("reserved Anchor provider already exists; recover the previous operation first")
	}
	after := []any{g.Model, provider, "disabled", map[string]any{
		"name": "Anchor (" + b.ID + ")", "base_url": g.BaseURL, "wire_api": "responses", "supports_websockets": false,
		"request_max_retries": codexRetries(g), "stream_max_retries": codexRetries(g),
		"auth": map[string]any{"command": b.Executable, "args": []string{"--data-dir", g.DataDir, "agent", "token", b.ID}, "timeout_ms": 5000, "refresh_interval_ms": 300000},
	}}
	if catalogPath != "" {
		after = append(after, catalogPath)
	}
	p := Plan{BindingID: b.ID, ExpectedRevision: b.Revision, Model: g.Model, GatewayURL: g.BaseURL, CredentialMethod: "command-backed binding token (not embedded)", AdapterVersion: "1", Path: path, Fingerprint: fingerprint(raw, exists), Fields: paths, Reload: "restart_required", Verification: "configuration_only; effective Agent connection unknown", original: raw, existed: exists}
	p.ModelScope = b.ModelScope
	p.AvailableModels = modelIDs
	p.CatalogPath = catalogPath
	p.CatalogVersion = g.CatalogVersion
	if catalogPath != "" {
		p.configFingerprint = p.Fingerprint
		p.catalog = &catalogArtifact{Path: catalogPath, Written: catalog}
		p.Fingerprint = fingerprint(append(append([]byte(p.configFingerprint+"\n"), catalog...), []byte(b.ModelScope+"\n"+g.CatalogVersion)...), true)
	}
	for i, path := range paths {
		p.Edits = append(p.Edits, Edit{Path: path, Before: values[i], After: Value{true, normalize(after[i])}})
	}
	if _, e = editor.Patch(raw, p.Edits); e != nil {
		return Plan{}, e
	}
	return p, nil
}
func (Codex) Verify(b state.Binding, g GatewayBinding) (Inspection, error) {
	out, e := (Codex{}).Inspect(b)
	if e != nil {
		return out, e
	}
	raw, _, e := readConfig(b.ConfigRoot, out.ConfigPath)
	if e != nil {
		return out, e
	}
	provider := "anchor_" + strings.NewReplacer("-", "_", ".", "_").Replace(b.ID)
	m, e := document(raw)
	if e != nil {
		return out, e
	}
	v := at(m, []string{"model_providers", provider})
	table, ok := v.Data.(map[string]any)
	if m["model"] != g.Model || m["model_provider"] != provider || !ok || table["base_url"] != g.BaseURL {
		return out, withDiagnostic(ErrChanged, "verify_configuration", "managed_field_mismatch", "codex.routing")
	}
	for _, field := range []string{"request_max_retries", "stream_max_retries"} {
		if !equal(Value{Exists: true, Data: table[field]}, Value{Exists: true, Data: int64(codexRetries(g))}) {
			return out, withDiagnostic(ErrChanged, "verify_configuration", "managed_field_mismatch", "codex.retries")
		}
	}
	if len(g.Models) > 0 && (b.ModelScope == "all-mapped" || g.CatalogVersion != "") {
		path := filepath.Join(b.ConfigRoot, catalogName(b.ID))
		if m["model_catalog_json"] != path {
			return out, withDiagnostic(ErrChanged, "verify_configuration", "managed_field_mismatch", "model_catalog_json")
		}
		raw, exists, err := readConfig(b.ConfigRoot, path)
		if err != nil {
			return out, err
		}
		expected, _, err := buildCatalogTemplates(g.Models, g.CatalogVersion, g.CatalogTemplates)
		if err != nil {
			return out, err
		}
		if !exists || !bytes.Equal(raw, expected) {
			return out, withDiagnostic(ErrChanged, "verify_catalog", "catalog_mismatch", "model_catalog_json")
		}
	}
	return out, nil
}
func ValidateTarget(b *state.Binding) error {
	if e := validateTargetIdentity(b); e != nil {
		return e
	}
	_, e := AdapterFor(b.AdapterID).Inspect(*b)
	return e
}

func validateTargetIdentity(b *state.Binding) error {
	root, e := filepath.Abs(b.ConfigRoot)
	if e != nil {
		return e
	}
	root = filepath.Clean(root)
	b.ConfigRoot = root
	if e = b.Validate(); e != nil {
		return e
	}
	st, e := os.Stat(b.Executable)
	if e != nil || !st.Mode().IsRegular() {
		return errors.New("anchor executable must be an existing stable binary")
	}
	return nil
}

func codexRetries(g GatewayBinding) int {
	if g.CodexRetries != nil {
		return *g.CodexRetries
	}
	return state.DefaultCodexRetries
}
