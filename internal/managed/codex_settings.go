package managed

import "github.com/candyflosssss/Anchor/internal/state"

type CodexSettingsState struct {
	Settings  state.CodexSettings `json:"settings"`
	NeedsSync []string            `json:"needs_sync"`
}

func (m *Manager) CodexSettings() (CodexSettingsState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.codexSettings()
}
func (m *Manager) codexSettings() (CodexSettingsState, error) {
	settings, err := m.Store.CodexSettings()
	out := CodexSettingsState{Settings: settings, NeedsSync: []string{}}
	if err != nil {
		return out, err
	}
	bindings, err := m.Store.Bindings()
	if err != nil {
		return out, err
	}
	for _, b := range bindings {
		if !b.Enabled || b.AdapterID != "codex" {
			continue
		}
		if r, ok := m.last[b.ID]; ok && r.Status != "applied" {
			out.NeedsSync = append(out.NeedsSync, b.ID)
		}
	}
	return out, nil
}
func (m *Manager) SaveCodexSettings(in state.SaveCodexSettings) (CodexSettingsState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping || !m.ready {
		return CodexSettingsState{}, state.ErrConflict
	}
	if _, err := m.Store.SaveCodexSettings(in); err != nil {
		return CodexSettingsState{}, err
	}
	// A committed preference remains saved even if a target cannot be updated.
	// Reconcile preserves external edits; only explicit agent sync reclaims them.
	results, err := m.syncMappings("codex")
	if err != nil {
		bindings, _ := m.Store.Bindings()
		for _, b := range bindings {
			if b.Enabled && b.AdapterID == "codex" {
				m.last[b.ID] = Result{BindingID: b.ID, Status: "needs_sync"}
			}
		}
	}
	for _, r := range results {
		if r.Status != "applied" {
			m.last[r.BindingID] = r
		}
	}
	return m.codexSettings()
}
