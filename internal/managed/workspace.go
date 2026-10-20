package managed

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/candyflosssss/Anchor/internal/state"
)

type AgentModel struct {
	ID               string   `json:"id"`
	Tools            bool     `json:"tools"`
	Streaming        bool     `json:"streaming"`
	UpstreamModels   []string `json:"upstream_models"`
	AvailableTargets []string `json:"available_targets"`
}
type ManagedAgent struct {
	ID              string            `json:"id"`
	AdapterID       string            `json:"adapter_id"`
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
	Rebuild          bool              `json:"rebuild,omitempty"`
	TargetID         string            `json:"target_id"`
	ModelScope       string            `json:"model_scope"`
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
	known, err := m.Store.Bindings()
	if err != nil {
		return out, err
	}
	for i := range targets {
		if targets[i].Status != "config_unsupported" {
			continue
		}
		for _, b := range known {
			if b.AdapterID == targets[i].Agent && sameRoot(b.ConfigRoot, targets[i].ConfigRoot) && m.codexRepairable(b) {
				targets[i].Status = "ready"
				break
			}
		}
	}
	out.Targets = targets
	// Preview the same automatic policy used by Enroll and live authorization.
	// Availability belongs to the core, including shared metadata and enabled
	// provider checks; clients must not reconstruct it from model names.
	availableTargets := map[string][]string{}
	for _, target := range targets {
		if !target.Supported || (target.Status != "ready" && !(target.Agent == "codex" && target.Status == "config_unsupported")) {
			continue
		}
		available, err := m.Store.BindingModels(state.Binding{ModelScope: "all-mapped"})
		if err != nil {
			return out, err
		}
		for _, model := range available {
			availableTargets[model.ID] = append(availableTargets[model.ID], target.ID)
		}
	}
	models, err := m.Store.Models()
	if err != nil {
		return out, err
	}
	for _, model := range models {
		row := AgentModel{ID: model.ID, Tools: model.Tools, Streaming: model.Streaming, UpstreamModels: []string{}, AvailableTargets: []string{}}
		row.AvailableTargets = append(row.AvailableTargets, availableTargets[model.ID]...)
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
		row := ManagedAgent{ID: b.ID, AdapterID: b.AdapterID, ConfigRoot: b.ConfigRoot, Model: b.Model, Enabled: b.Enabled, Revision: b.Revision, ModelScope: b.ModelScope, CatalogVersion: b.CodexVersion, Status: statusMap[b.ID], AvailableModels: []string{}, CatalogPath: filepath.Join(b.ConfigRoot, catalogName(b.ID)), ModelTemplates: map[string]string{}}
		if b.AdapterID != "codex" {
			row.CatalogPath = ""
		}
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
			} else if _, err := AdapterFor(b.AdapterID).Verify(b, g); err != nil {
				row.Status = "needs_sync"
			}
		}
		out.Bindings = append(out.Bindings, row)
	}
	return out, nil
}

// Enroll persists an explicitly selected target and local catalog snapshot.
// Existing provider mappings are untouched. Updates explicitly reapply the
// submitted settings and current mappings, preserving external recovery values.
// File writes remain Engine-owned.
func (m *Manager) Enroll(env Environment, in EnrollAgent) (state.Binding, error) {
	if !in.Approved || !state.ValidModelID(in.Model) || in.ExpectedRevision < 0 {
		return state.Binding{}, state.ErrInvalid
	}
	if in.Rebuild && (in.ModelScope != "all-mapped" || len(in.ModelTemplates) != 0) {
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
	if target == nil || !target.Supported || (target.Status != "ready" && target.Status != "config_unsupported") {
		return state.Binding{}, state.ErrInvalid
	}
	if in.Rebuild && target.Agent != "codex" {
		return state.Binding{}, state.ErrInvalid
	}
	scope := in.ModelScope
	if scope == "" {
		scope = "all-compatible"
		if target.Agent != "codex" {
			scope = "all-mapped"
		}
	}
	if scope != "all-compatible" && scope != "all-mapped" {
		return state.Binding{}, state.ErrInvalid
	}
	b := state.Binding{ID: target.ID, AdapterID: target.Agent, AdapterVersion: "1", ConfigRoot: target.ConfigRoot, Model: in.Model, ModelScope: scope, Executable: env.Executable, Enabled: true}
	if scope == "all-compatible" {
		version, catalog, e := localCatalog(target.ConfigRoot)
		if e != nil {
			return b, e
		}
		b.CodexVersion = version
		b.CodexCatalog = catalog
		b.ModelTemplates = in.ModelTemplates
	} else if len(in.ModelTemplates) > 0 {
		return b, state.ErrInvalid
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
			if old.AdapterID != b.AdapterID {
				if state.BindingRootsConflict(old, b) {
					return b, state.ErrConflict
				}
				continue
			}
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
	repairable := b.Revision > 0 && m.codexRepairable(b)
	if target.Status != "ready" && !repairable && !in.Rebuild {
		return b, state.ErrInvalid
	}
	validate := ValidateTarget
	if repairable || in.Rebuild {
		validate = validateTargetIdentity
	}
	if err := validate(&b); err != nil {
		return b, err
	}
	g, err := m.gatewayFor(b, !in.Rebuild)
	if err != nil {
		return b, err
	}
	if b.ModelScope == "all-compatible" {
		if _, _, err := buildCatalog(g.Models, g.CatalogVersion); err != nil {
			return b, err
		}
	}
	reapply := false
	if in.Rebuild {
		if !m.ready {
			return b, errors.New("gateway is not ready")
		}
		if err := m.Engine.validateRebuild(b); err != nil {
			return b, err
		}
	}
	if b.Revision == 0 {
		b, err = m.Store.AddBinding(b)
	} else {
		if !repairable && !in.Rebuild {
			if _, err := AdapterFor(b.AdapterID).Inspect(b); err != nil {
				return b, err
			}
		}
		current, e := m.Engine.load(b.ID)
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return b, e
		}
		// A live explicit update uses the submitted policy, not the previously
		// saved default model. Reapply keeps external values as recovery baselines
		// without temporarily publishing a restored configuration. Incomplete
		// writes still require ordinary recovery; they are never blindly reclaimed.
		reapply = m.ready && e == nil && current.Previous == nil && (current.Status == "applied" || current.Status == "conflict")
		if !reapply && !in.Rebuild {
			restored, e := m.Engine.Recover(b.ID)
			if e != nil {
				return b, e
			}
			if restored.Status == "conflict" {
				return b, withDiagnostic(ErrChanged, "recovery", "recovery_conflict", "")
			}
		}
		b, err = m.Store.UpdateBindingCatalog(b, in.ExpectedRevision)
	}
	if err != nil {
		return b, err
	}
	delete(m.last, b.ID)
	if m.ready {
		var r Result
		var e error
		if in.Rebuild {
			g, e = m.gatewayFor(b, false)
			if e == nil {
				r, e = m.Engine.RebuildCodex(b, g)
			}
		} else if reapply {
			g, e = m.gateway(b)
			if e == nil {
				r, e = m.Engine.Reapply(b, g)
			}
		} else {
			r, e = m.apply(b.ID, b.Revision, "")
		}
		if e != nil {
			if r.Status != "recovery_pending" {
				r = Result{BindingID: b.ID, Status: "apply_failed"}
			}
			m.last[b.ID] = r
			return b, e
		}
		m.last[b.ID] = r
	}
	return b, nil
}

func sameRoot(a, b string) bool { return sameCatalogPath(a, filepath.Clean(b)) }

// Refresh explicitly reclaims approved fields while preserving external values
// for later recovery. Background SyncMappings retains strict drift checks.
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
	if current, e := m.Engine.load(id); e == nil && (current.Status == "applied" || current.Status == "conflict") {
		r, e := m.Engine.Reapply(b, g)
		if e != nil {
			m.last[id] = r
		} else {
			delete(m.last, id)
		}
		return r, e
	}
	// Validate the next catalog before restoring the working configuration.
	if len(g.Models) > 0 && g.CatalogVersion != "" {
		if _, _, err = buildCatalog(g.Models, g.CatalogVersion); err != nil {
			return Result{}, err
		}
	}
	if _, err := AdapterFor(b.AdapterID).Inspect(b); err != nil {
		return Result{}, err
	}
	r, err := m.Engine.Recover(id)
	if err != nil {
		return r, err
	}
	if r.Status == "conflict" {
		return r, withDiagnostic(ErrChanged, "recovery", "recovery_conflict", "")
	}
	r, err = m.apply(id, revision, "")
	if err != nil {
		m.last[id] = Result{BindingID: id, Status: "apply_failed"}
	} else {
		delete(m.last, id)
	}
	return r, err
}
