package managed

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/candyflosssss/Anchor/internal/state"
)

// The app owns configLibrary and its active-profile pointer. Anchor edits only
// the already-applied local profile, never MDM, bootstrap or the pointer itself.
func inspectClaudeDesktop(b state.Binding) (Inspection, error) {
	if filepath.Base(b.ConfigRoot) != "configLibrary" {
		return Inspection{}, ErrUnsafe
	}
	if claudeDesktopPolicyPresent(b.ConfigRoot) {
		return Inspection{}, withDiagnostic(ErrUnsafe, "inspect_configuration", "desktop_policy_present", "")
	}
	meta, exists, err := readConfig(b.ConfigRoot, filepath.Join(b.ConfigRoot, "_meta.json"))
	if err != nil || !exists {
		return Inspection{}, ErrUnsafe
	}
	if _, err := (StructuredEditor{}).parse(meta); err != nil {
		return Inspection{}, err
	}
	var selection struct {
		AppliedID string `json:"appliedId"`
		Entries   []struct {
			ID string `json:"id"`
		} `json:"entries"`
		HybridPointer json.RawMessage `json:"hybridPointer"`
	}
	if json.Unmarshal(meta, &selection) != nil || !state.ValidClaudeDesktopProfile(selection.AppliedID) || (len(selection.HybridPointer) > 0 && string(selection.HybridPointer) != "null") {
		return Inspection{}, ErrUnsafe
	}
	found := 0
	for _, entry := range selection.Entries {
		if entry.ID == selection.AppliedID {
			found++
		}
	}
	if found != 1 {
		return Inspection{}, ErrUnsafe
	}
	if b.ClaudeDesktopProfile != "" && selection.AppliedID != b.ClaudeDesktopProfile {
		return Inspection{}, withDiagnostic(ErrChanged, "inspect_configuration", "desktop_profile_changed", "")
	}
	path := filepath.Join(b.ConfigRoot, selection.AppliedID+".json")
	raw, exists, err := readConfig(b.ConfigRoot, path)
	if err != nil || !exists {
		return Inspection{}, ErrUnsafe
	}
	root, err := (StructuredEditor{}).parse(raw)
	if err != nil {
		return Inspection{}, err
	}
	// Remote configuration replaces local values. Do not pretend to override it.
	if node := nodeKey(root, "bootstrapUrl"); node != nil && node.Tag != "!!null" && (node.Tag != "!!str" || node.Value != "") {
		return Inspection{}, ErrUnsafe
	}
	return Inspection{AdapterID: "claude-desktop", AdapterVersion: "1", ConfigPath: path, Exists: true, Support: "configuration_only", EffectiveConnection: "unknown", Reload: "restart_required", Limitations: []string{
		"local applied third-party inference profile only; organization policy and bootstrap profiles are not managed",
		"affects Claude Desktop Chat, Cowork and Code; fully quit and relaunch Desktop after apply or restore",
		"CLI settings and shared skills/hooks/MCP are not modified; Desktop manages its own engine version",
		"configuration only; no real client, upstream, WSL or SSH verification; remote sessions cannot reach this device's localhost",
	}}, nil
}

func claudeDesktopDesired(b state.Binding, g GatewayBinding) ([][]string, []any, error) {
	models := []any{}
	// Desktop uses the first entry as its default, independent of store order.
	appendModel := func(model state.Model) {
		label := model.DisplayName
		if label == "" {
			label = model.ID
		}
		models = append(models, map[string]any{"name": model.ID, "labelOverride": label})
	}
	found := false
	for _, model := range g.Models {
		if model.ID == b.Model {
			appendModel(model)
			found = true
			break
		}
	}
	if !found {
		return nil, nil, ErrModelUnavailable
	}
	for _, model := range g.Models {
		if model.ID != b.Model {
			appendModel(model)
		}
	}
	paths := [][]string{{"inferenceProvider"}, {"inferenceGatewayBaseUrl"}, {"inferenceGatewayApiKey"}, {"inferenceGatewayAuthScheme"}, {"inferenceCredentialKind"}, {"inferenceModels"}, {"modelDiscoveryEnabled"}}
	values := []any{"gateway", strings.TrimSuffix(g.BaseURL, "/v1"), g.Token, "bearer", "static", models, false}
	// Prevent old credentials/headers from taking precedence or being sent to
	// Anchor. The engine keeps every previous value in its protected journal.
	for _, key := range []string{"inferenceCredentialHelper", "inferenceCredentialHelperWindows", "inferenceCredentialHelperArgs", "inferenceGatewayOidc", "inferenceIdpOidc", "inferenceCustomHeaders"} {
		paths = append(paths, []string{key})
		values = append(values, nil)
	}
	return paths, values, nil
}

func claudeDesktopDataRoot(home, appData, localAppData, goos string) string {
	switch goos {
	case "windows":
		if localAppData == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "Claude-3p")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude-3p")
	default:
		if appData == "" {
			appData = filepath.Join(home, ".config")
		}
		return filepath.Join(appData, "Claude-3p")
	}
}

func filePresentOrUnreadable(path string) bool {
	_, err := os.Lstat(path)
	return !errors.Is(err, os.ErrNotExist)
}
