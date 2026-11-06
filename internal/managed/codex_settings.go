package managed

import (
	"encoding/json"

	"github.com/candyflosssss/Anchor/internal/state"
)

// CodexCompactionStatus describes file configuration, never client reload or a
// successful compaction request. Values come from the same catalog exporter.
type CodexCompactionStatus struct {
	BindingID     string `json:"binding_id"`
	ModelID       string `json:"model_id"`
	IsDefault     bool   `json:"is_default"`
	ContextWindow *int64 `json:"context_window"`
	TokenLimit    *int64 `json:"token_limit"`
	Status        string `json:"status"`
}

type CodexSettingsState struct {
	Settings   state.CodexSettings     `json:"settings"`
	NeedsSync  []string                `json:"needs_sync"`
	Compaction []CodexCompactionStatus `json:"compaction"`
}

func (m *Manager) CodexSettings() (CodexSettingsState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.codexSettings()
}
func (m *Manager) codexSettings() (CodexSettingsState, error) {
	settings, err := m.Store.CodexSettings()
	out := CodexSettingsState{Settings: settings, NeedsSync: []string{}, Compaction: []CodexCompactionStatus{}}
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
		rows, needsSync := m.codexCompaction(b, settings)
		out.Compaction = append(out.Compaction, rows...)
		if needsSync {
			out.NeedsSync = append(out.NeedsSync, b.ID)
		}
	}
	return out, nil
}

// Caller holds m.mu. This is a read-only inspection: no model defaults are
// persisted and external edits are never reconciled by a settings GET.
func (m *Manager) codexCompaction(b state.Binding, settings state.CodexSettings) ([]CodexCompactionStatus, bool) {
	fallback := []CodexCompactionStatus{{BindingID: b.ID, ModelID: b.Model, IsDefault: true, Status: "needs_sync"}}
	g, err := m.gateway(b)
	if err != nil {
		return fallback, true
	}
	status, err := m.Engine.Status(b.ID)
	fileStatus := "written"
	needsSync := false
	if err != nil {
		fileStatus, needsSync = "needs_sync", true
	} else if status.Status == "unmanaged" || status.Status == "restored" {
		fileStatus = "unmanaged"
	} else if status.Status != "applied" {
		fileStatus, needsSync = "needs_sync", true
	} else if _, err := (Codex{}).Verify(b, g); err != nil {
		fileStatus, needsSync = "needs_sync", true
	}
	// Keep failed background operations visible even when the old files still
	// match. An explicit successful sync clears m.last through the existing path.
	if previous, ok := m.last[b.ID]; ok && previous.Status != "applied" {
		fileStatus, needsSync = "needs_sync", true
	}
	if !settings.OverrideModelCatalog {
		fallback[0].Status = fileStatus
		if fileStatus == "written" {
			fallback[0].Status = "disabled"
		}
		return fallback, needsSync
	}
	rows := []CodexCompactionStatus{}
	if len(g.Models) == 0 || (b.ModelScope != "all-mapped" && g.CatalogVersion == "") {
		fallback[0].Status = fileStatus
		if fileStatus == "written" {
			fallback[0].Status = "unsupported"
			if !settings.AutoCompact {
				fallback[0].Status = "disabled"
			}
		}
		return fallback, needsSync
	}
	raw, _, err := buildGatewayCatalog(g)
	if err != nil {
		return fallback, true
	}
	var catalog struct {
		Models []struct {
			Slug    string `json:"slug"`
			Context *int64 `json:"context_window"`
			Limit   *int64 `json:"auto_compact_token_limit"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return fallback, true
	}
	for _, entry := range catalog.Models {
		row := CodexCompactionStatus{BindingID: b.ID, ModelID: entry.Slug, IsDefault: entry.Slug == b.Model, Status: fileStatus}
		// Only a verified on-disk value is reported as configured. Planned values
		// are deliberately withheld for stale, external or unmanaged files.
		if fileStatus == "written" {
			row.ContextWindow = entry.Context
			if settings.AutoCompact {
				row.TokenLimit = entry.Limit
				if entry.Context == nil || *entry.Context <= 0 || entry.Limit == nil {
					row.Status = "missing_context"
					row.TokenLimit = nil
				}
			}
		}
		if !settings.AutoCompact && fileStatus == "written" {
			row.Status = "disabled"
		}
		rows = append(rows, row)
	}
	return rows, needsSync
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
