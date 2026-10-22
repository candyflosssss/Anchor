package managed

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/candyflosssss/Anchor/internal/state"
)

// AdapterFor keeps planning, verification and lifecycle on the same adapter.
func AdapterFor(id string) Adapter {
	switch id {
	case "codex":
		return Codex{}
	case "claude":
		return StructuredAgent{}
	case "claude-desktop":
		return StructuredAgent{Desktop: true}
	case "dsh":
		return StructuredAgent{DSH: true}
	case "dsh-desktop":
		return StructuredAgent{DSH: true, Desktop: true}
	}
	return unsupportedAdapter{}
}

type unsupportedAdapter struct{ Codex }

func (unsupportedAdapter) Inspect(state.Binding) (Inspection, error) {
	return Inspection{}, state.ErrInvalid
}
func (unsupportedAdapter) Plan(state.Binding, GatewayBinding) (Plan, error) {
	return Plan{}, state.ErrInvalid
}
func (unsupportedAdapter) Verify(state.Binding, GatewayBinding) (Inspection, error) {
	return Inspection{}, state.ErrInvalid
}

type StructuredAgent struct{ DSH, Desktop bool }

func (a StructuredAgent) profile() string {
	if a.Desktop {
		return "desktop"
	}
	return "web"
}

func (a StructuredAgent) path(b state.Binding) string {
	if a.DSH {
		return filepath.Join(b.ConfigRoot, "profiles", a.profile(), "cordis.patch.yml")
	}
	return filepath.Join(b.ConfigRoot, "settings.json")
}
func (a StructuredAgent) Discover(b state.Binding) (Inspection, error) { return a.Inspect(b) }
func (a StructuredAgent) Inspect(b state.Binding) (Inspection, error) {
	if a.Desktop && !a.DSH {
		return inspectClaudeDesktop(b)
	}
	path := a.path(b)
	raw, exists, err := readConfig(filepath.Dir(path), path)
	if err != nil {
		return Inspection{}, err
	}
	editor := StructuredEditor{DSH: a.DSH}
	root, err := editor.parse(raw)
	if err != nil {
		return Inspection{}, err
	}
	if !a.DSH {
		if _, err := editor.Values(raw, [][]string{{"env", "ANTHROPIC_MODEL"}}); err != nil {
			return Inspection{}, err
		}
	}
	id := "claude"
	limits := []string{"CLI user settings only; Desktop inference uses a separate adapter; project, managed and CLI overrides are not probed", "modelPicker requires Claude Code 2.1.242 or later; older clients may show only the custom default and recognizable Claude IDs", "configuration verification only; restart Claude Code after changes; all tier and subagent aliases use the selected default"}
	reload := "restart_required"
	if a.DSH {
		id = "dsh"
		reload = "new_session_required"
		if a.Desktop {
			id = "dsh-desktop"
		}
		limits = []string{a.profile() + " profile only; home patches and runtime overrides are not probed", "configuration only; with HMR disabled fully quit and restart the client (closing the Desktop window may only hide it); start a new session to use the changed default model; no real upstream verification"}
		for _, row := range root.Content {
			if insert := nodeKey(row, "insert"); insert != nil {
				return Inspection{}, errors.New("DSH insert patches are not supported by this adapter revision")
			}
		}
	}
	return Inspection{AdapterID: id, AdapterVersion: "1", ConfigPath: path, Exists: exists, Support: "configuration_only", EffectiveConnection: "unknown", Reload: reload, Limitations: limits}, nil
}
func (a StructuredAgent) desired(b state.Binding, g GatewayBinding, raw []byte) ([][]string, []any, error) {
	if a.Desktop && !a.DSH {
		return claudeDesktopDesired(b, g)
	}
	ids := []string{}
	models := []any{}
	var defaultReasoning any
	for _, m := range g.Models {
		ids = append(ids, m.ID)
		if a.DSH {
			entry, reasoning, err := dshModel(m)
			if err != nil {
				return nil, nil, err
			}
			models = append(models, entry)
			if m.ID == b.Model {
				defaultReasoning = reasoning
			}
		}
	}
	if len(ids) == 0 {
		ids = append(ids, b.Model)
		models = append(models, map[string]any{"id": b.Model, "name": b.Model})
	}
	if !a.DSH {
		paths := [][]string{{"model"}, {"availableModels"}, {"env", "ANTHROPIC_BASE_URL"}, {"env", "ANTHROPIC_AUTH_TOKEN"}, {"env", "ANTHROPIC_API_KEY"}, {"env", "ANTHROPIC_MODEL"}, {"env", "ANTHROPIC_DEFAULT_SONNET_MODEL"}, {"env", "ANTHROPIC_DEFAULT_OPUS_MODEL"}, {"env", "ANTHROPIC_DEFAULT_HAIKU_MODEL"}, {"env", "CLAUDE_CODE_SUBAGENT_MODEL"}, {"env", "CLAUDE_CODE_USE_BEDROCK"}, {"env", "CLAUDE_CODE_USE_VERTEX"}, {"env", "CLAUDE_CODE_USE_FOUNDRY"}, {"apiKeyHelper"}, {"env", "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"}, {"env", "ANTHROPIC_CUSTOM_MODEL_OPTION"}}
		options := []any{}
		for _, id := range ids {
			label := id
			for _, model := range g.Models {
				if model.ID == id && model.DisplayName != "" {
					label = model.DisplayName
					break
				}
			}
			options = append(options, map[string]any{"model": id, "label": label})
		}
		// Explicit rows also expose aliases that gateway discovery filters out.
		paths = append(paths, []string{"modelPicker"}, []string{"env", "CLAUDE_CODE_USE_ANTHROPIC_AWS"})
		return paths, []any{b.Model, ids, strings.TrimSuffix(g.BaseURL, "/v1"), g.Token, "", b.Model, b.Model, b.Model, b.Model, b.Model, "0", "0", "0", nil, "1", b.Model, map[string]any{"options": options, "replaceBuiltInOptions": true}, "0"}, nil
	}
	provider := "anchor-" + b.ID
	route := map[string]any{"displayName": "Anchor", "api": "openai-completions", "baseURL": g.BaseURL, "headers": map[string]any{"Authorization": "Bearer " + g.Token}, "models": models, "retryPolicy": map[string]any{"mode": "normal", "maxRetries": 0}}
	editor := StructuredEditor{DSH: true}
	root, err := editor.parse(raw)
	if err != nil {
		return nil, nil, err
	}
	paths := [][]string{}
	after := []any{}
	// Own a whole row only when we create it. Existing rows receive narrow edits,
	// retaining their other providers and plugin settings during apply/recovery.
	for _, id := range []string{"llm-pi-ai", "agent-default-model"} {
		var rowFound bool
		for _, row := range root.Content {
			if n := nodeKey(row, "id"); n != nil && n.Value == id {
				rowFound = true
			}
		}
		if !rowFound {
			config := map[string]any{"provider": provider, "model": b.Model}
			if defaultReasoning != nil {
				config["reasoningEffort"] = defaultReasoning
			}
			if id == "llm-pi-ai" {
				config = map[string]any{"providers": map[string]any{provider: route}}
			}
			paths = append(paths, []string{id})
			after = append(after, map[string]any{"id": id, "config": config})
			continue
		}
		if id == "llm-pi-ai" {
			paths = append(paths, []string{id, "config", "providers", provider})
			after = append(after, route)
		} else {
			// Always own the default effort, including its absence. This clears
			// a stale user/default-model effort and preserves its recovery baseline.
			paths = append(paths, []string{id, "config", "provider"}, []string{id, "config", "model"}, []string{id, "config", "reasoningEffort"})
			after = append(after, provider, b.Model, defaultReasoning)
		}
	}
	return paths, after, nil
}
func (a StructuredAgent) Plan(b state.Binding, g GatewayBinding) (Plan, error) {
	if err := b.Validate(); err != nil {
		return Plan{}, err
	}
	if !g.Ready || g.Token == "" {
		return Plan{}, errors.New("gateway and binding credential must be ready")
	}
	inspection, err := a.Inspect(b)
	if err != nil {
		return Plan{}, err
	}
	raw, exists, err := readConfig(filepath.Dir(inspection.ConfigPath), inspection.ConfigPath)
	if err != nil {
		return Plan{}, err
	}
	return a.planFrom(b, g, raw, exists, "")
}

func (a StructuredAgent) planFrom(b state.Binding, g GatewayBinding, raw []byte, exists bool, _ string) (Plan, error) {
	if err := b.Validate(); err != nil {
		return Plan{}, err
	}
	if !g.Ready {
		return Plan{}, ErrUnsafe
	}
	inspection, err := a.Inspect(b)
	if err != nil {
		return Plan{}, err
	}
	paths, after, err := a.desired(b, g, raw)
	if err != nil {
		return Plan{}, err
	}
	values, err := (StructuredEditor{DSH: a.DSH}).Values(raw, paths)
	if err != nil {
		return Plan{}, err
	}
	if a.DSH {
		for i, path := range paths {
			if len(path) == 4 && path[0] == "llm-pi-ai" && values[i].Exists {
				return Plan{}, errors.New("reserved Anchor provider already exists; recover the previous operation first")
			}
		}
	}
	p := Plan{BindingID: b.ID, ExpectedRevision: b.Revision, Model: b.Model, ModelScope: b.ModelScope, GatewayURL: g.BaseURL, CredentialMethod: "scoped binding token in owner-only configuration", AdapterVersion: "1", Path: inspection.ConfigPath, Fingerprint: fingerprint(raw, exists), Fields: paths, Reload: inspection.Reload, Verification: "configuration only", original: raw, existed: exists}
	if a.Desktop && !a.DSH {
		b.ClaudeDesktopProfile = strings.TrimSuffix(filepath.Base(inspection.ConfigPath), ".json")
		p.check = func() error { _, err := inspectClaudeDesktop(b); return err }
	}
	for i, path := range paths {
		p.Edits = append(p.Edits, Edit{Path: path, Before: values[i], After: Value{Exists: after[i] != nil, Data: normalize(after[i])}})
	}
	for _, model := range g.Models {
		p.AvailableModels = append(p.AvailableModels, model.ID)
	}
	return p, nil
}
func (a StructuredAgent) Verify(b state.Binding, g GatewayBinding) (Inspection, error) {
	inspection, err := a.Inspect(b)
	if err != nil {
		return inspection, err
	}
	raw, _, err := readConfig(filepath.Dir(inspection.ConfigPath), inspection.ConfigPath)
	if err != nil {
		return inspection, err
	}
	paths, after, err := a.desired(b, g, raw)
	if err != nil {
		return inspection, err
	}
	values, err := (StructuredEditor{DSH: a.DSH}).Values(raw, paths)
	if err != nil {
		return inspection, err
	}
	for i, value := range values {
		if !equal(value, Value{Exists: after[i] != nil, Data: normalize(after[i])}) {
			return inspection, withDiagnostic(ErrChanged, "verify_configuration", "managed_field_mismatch", diagnosticField(paths[i]))
		}
	}
	return inspection, nil
}
