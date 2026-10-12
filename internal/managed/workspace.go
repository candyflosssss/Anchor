package managed

import (
	"errors"
	"path/filepath"

	"github.com/candyflosssss/Anchor/internal/state"
)

type AgentModel struct {
	ID             string   `json:"id"`
	Tools          bool     `json:"tools"`
	Streaming      bool     `json:"streaming"`
	UpstreamModels []string `json:"upstream_models"`
}
type ManagedAgent struct {
	ID              string            `json:"id"`
	ConfigRoot      string            `json:"config_root"`
	Model           string            `json:"model"`
	Enabled         bool              `json:"enabled"`
	Revision        int64             `json:"revision"`
	ModelScope      string            `json:"model_scope"`
	CatalogVersion  string            `json:"catalog_version"`
	Status          string            `json:"status"`
	AvailableModels []string          `json:"available_models"`
	CatalogPath     string            `json:"catalog_path"`
	ModelTemplates  map[string]string `json:"model_templates"`
}
type AgentWorkspace struct {
	Targets  []AgentTarget  `json:"targets"`
	Bindings []ManagedAgent `json:"bindings"`
	Models   []AgentModel   `json:"models"`
}
type EnrollAgent struct {
	TargetID         string            `json:"target_id"`
	Model            string            `json:"model"`
	ExpectedRevision int64             `json:"expected_revision"`
	ModelTemplates   map[string]string `json:"model_templates"`
	Approved         bool              `json:"approved"`
}
type ManageAgent struct {
	BindingID        string `json:"binding_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Action           string `json:"action"`
}

func (m *Manager) Workspace(env Environment) (AgentWorkspace, error) {
	out := AgentWorkspace{Targets: []AgentTarget{}, Bindings: []ManagedAgent{}, Models: []AgentModel{}}
	targets, err := Discover(env)
	if err != nil {
		return out, err
	}
	out.Targets = targets
	models, err := m.Store.Models()
	if err != nil {
		return out, err
	}
	for _, model := range models {
		row := AgentModel{ID: model.ID, Tools: model.Tools, Streaming: model.Streaming, UpstreamModels: []string{}}
		for _, c := range model.Candidates {
			row.UpstreamModels = append(row.UpstreamModels, c.Model)
		}
		out.Models = append(out.Models, row)
	}
	statuses, err := m.Status()
	if err != nil {
		return out, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	statusMap := map[string]string{}
	for _, r := range statuses {
		statusMap[r.BindingID] = r.Status
	}
	bindings, err := m.Store.Bindings()
	if err != nil {
		return out, err
	}
	for _, b := range bindings {
		row := ManagedAgent{ID: b.ID, ConfigRoot: b.ConfigRoot, Model: b.Model, Enabled: b.Enabled, Revision: b.Revision, ModelScope: b.ModelScope, CatalogVersion: b.CodexVersion, Status: statusMap[b.ID], AvailableModels: []string{}, CatalogPath: filepath.Join(b.ConfigRoot, catalogName(b.ID)), ModelTemplates: map[string]string{}}
		for alias, source := range b.ModelTemplates {
			row.ModelTemplates[alias] = source
		}
		allowed, err := m.Store.BindingModels(b)
		if err != nil {
			return out, err
		}
		for _, model := range allowed {
			row.AvailableModels = append(row.AvailableModels, model.ID)
		}
		if row.Status == "applied" {
			// A journal records the last write, not proof that the current catalog is fresh.
			g, err := m.gateway(b)
			if err != nil {
				row.Status = "apply_failed"
			} else if _, err := (Codex{}).Verify(b, g); err != nil {
				row.Status = "needs_sync"
			}
		}
		out.Bindings = append(out.Bindings, row)
	}
	return out, nil
}

// Enroll persists an explicitly selected target and local catalog snapshot.
// Existing provider mappings are untouched. File writes remain Engine-owned.
func (m *Manager) Enroll(env Environment, in EnrollAgent) (state.Binding, error) {
	if !in.Approved || !state.ValidID(in.Model) || in.ExpectedRevision < 0 {
		return state.Binding{}, state.ErrInvalid
	}
	targets, err := Discover(env)
	if err != nil {
		return state.Binding{}, err
	}
	var target *AgentTarget
	for i := range targets {
		if targets[i].ID == in.TargetID {
			target = &targets[i]
			break
		}
	}
	if target == nil || !target.Supported || target.Status != "ready" {
		return state.Binding{}, state.ErrInvalid
	}
	version, catalog, err := localCatalog(target.ConfigRoot)
	if err != nil {
		return state.Binding{}, err
	}
	b := state.Binding{ID: target.ID, AdapterID: "codex", AdapterVersion: "1", ConfigRoot: target.ConfigRoot, Model: in.Model, ModelScope: "all-compatible", CodexVersion: version, Executable: env.Executable, Enabled: true, CodexCatalog: catalog, ModelTemplates: in.ModelTemplates}
	if err := ValidateTarget(&b); err != nil {
		return b, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping {
		return b, errors.New("management is stopping")
	}
	// Existing roots may have been registered under a CLI-chosen ID.
	bindings, err := m.Store.Bindings()
	if err != nil {
		return b, err
	}
	for _, old := range bindings {
		if sameRoot(old.ConfigRoot, b.ConfigRoot) {
			b.ID = old.ID
			b.ConfigRoot = old.ConfigRoot
			b.Executable = old.Executable
			b.Revision = old.Revision
			break
		}
	}
	if b.Revision != in.ExpectedRevision {
		return b, state.ErrConflict
	}
	g, err := m.gateway(b)
	if err != nil {
		return b, err
	}
	if _, _, err := buildCatalog(g.Models, g.CatalogVersion); err != nil {
		return b, err
	}
	if b.Revision == 0 {
		b, err = m.Store.AddBinding(b)
	} else {
		if _, err := (Codex{}).Inspect(b); err != nil {
			return b, err
		}
		restored, e := m.Engine.Recover(b.ID)
		if e != nil {
			return b, e
		}
		if restored.Status == "conflict" {
			return b, ErrChanged
		}
		b, err = m.Store.UpdateBindingCatalog(b, in.ExpectedRevision)
	}
	if err != nil {
		return b, err
	}
	delete(m.last, b.ID)
	if m.ready {
		r, e := m.apply(b.ID, b.Revision, "")
		if e != nil {
			m.last[b.ID] = Result{BindingID: b.ID, Status: "apply_failed"}
			return b, e
		}
		m.last[b.ID] = r
	}
	return b, nil
}

func sameRoot(a, b string) bool { return sameCatalogPath(a, filepath.Clean(b)) }

// Refresh uses conditional recovery before generating a fresh snapshot. It
// refuses third-party edits and never turns Anchor's own values into originals.
func (m *Manager) Refresh(id string, revision int64) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping || !m.ready {
		return Result{}, errors.New("gateway is not ready")
	}
	b, err := m.Store.Binding(id)
	if err != nil {
		return Result{}, err
	}
	if b.Revision != revision {
		return Result{}, state.ErrConflict
	}
	if !b.Enabled {
		return Result{}, errors.New("binding is disabled")
	}
	g, err := m.gateway(b)
	if err != nil {
		return Result{}, err
	}
	// Validate the next catalog before restoring the working configuration.
	if len(g.Models) > 0 && g.CatalogVersion != "" {
		if _, _, err = buildCatalog(g.Models, g.CatalogVersion); err != nil {
			return Result{}, err
		}
	}
	if _, err := (Codex{}).Inspect(b); err != nil {
		return Result{}, err
	}
	r, err := m.Engine.Recover(id)
	if err != nil {
		return r, err
	}
	if r.Status == "conflict" {
		return r, ErrChanged
	}
	r, err = m.apply(id, revision, "")
	if err != nil {
		m.last[id] = Result{BindingID: id, Status: "apply_failed"}
	} else {
		delete(m.last, id)
	}
	return r, err
}
