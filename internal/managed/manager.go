package managed

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/candyflosssss/Anchor/internal/state"
)

type Manager struct {
	Store    *state.Store
	Engine   *Engine
	mu       sync.Mutex
	ready    bool
	stopping bool
	last     map[string]Result
}

func NewManager(s *state.Store) *Manager {
	return &Manager{Store: s, Engine: NewEngine(s), last: map[string]Result{}}
}
func (m *Manager) gateway(b state.Binding) (GatewayBinding, error) {
	cfg, e := m.Store.Settings()
	if e != nil {
		return GatewayBinding{}, e
	}
	model, e := m.Store.Model(b.Model)
	if e != nil {
		if errors.Is(e, state.ErrNotFound) {
			return GatewayBinding{}, fmt.Errorf("%w: %w", ErrModelUnavailable, e)
		}
		return GatewayBinding{}, e
	}
	models, e := m.Store.BindingModels(b)
	if e != nil {
		return GatewayBinding{}, e
	}
	model.Codex = state.BindingMetadata(b, model)
	version := b.CodexVersion
	if version == "" && model.Codex != nil {
		version = model.Codex.Version
	}
	if b.ModelScope == "all-compatible" || b.ModelScope == "all-mapped" {
		found := false
		for _, allowed := range models {
			if allowed.ID == b.Model {
				found = true
			}
		}
		if !found {
			return GatewayBinding{}, ErrModelUnavailable
		}
	} else if b.AdapterID == "codex" && model.Codex == nil {
		models = nil
	}
	token := ""
	if b.Revision > 0 && b.Enabled {
		token, e = m.Store.BindingToken(b.ID)
		if e != nil && !errors.Is(e, state.ErrNotFound) {
			return GatewayBinding{}, e
		}
	}
	if b.ModelScope == "all-mapped" {
		version = ""
	}
	var templates map[string]json.RawMessage
	if b.AdapterID == "codex" && b.ModelScope == "all-mapped" {
		var err error
		templates, err = m.codexTemplates(b)
		if err != nil {
			return GatewayBinding{}, fmt.Errorf("%w: %w", ErrCatalogInvalid, err)
		}
	}
	var retries *int
	if b.AdapterID == "codex" {
		retrySettings, err := m.Store.CodexSettings()
		if err != nil {
			return GatewayBinding{}, err
		}
		retries = &retrySettings.Retries
	}
	return GatewayBinding{CodexRetries: retries, Token: token, CatalogTemplates: templates, Models: models, CatalogVersion: version, BaseURL: "http://" + cfg.GatewayAddress + "/v1", Model: b.Model, DataDir: m.Store.Dir, Ready: m.ready && !m.stopping, Tools: model.Tools, Streaming: model.Streaming}, nil
}
func (m *Manager) Add(b state.Binding) (state.Binding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping {
		return b, errors.New("management is stopping")
	}
	if b.AdapterID == "codex" && b.ModelScope != "all-mapped" && b.CodexVersion == "" {
		model, err := m.Store.Model(b.Model)
		if err != nil {
			return b, err
		}
		if model.Codex != nil {
			b.CodexVersion = model.Codex.Version
		}
	}
	if e := ValidateTarget(&b); e != nil {
		return b, e
	}
	g, e := m.gateway(b)
	if e != nil {
		return b, e
	}
	if b.AdapterID == "codex" && b.ModelScope != "all-mapped" && (!g.Tools || !g.Streaming) {
		return b, errors.New("Codex model requires --tools and --streaming")
	}
	return m.Store.AddBinding(b)
}
func (m *Manager) Plan(id string) (Plan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, e := m.Store.Binding(id)
	if e != nil {
		return Plan{}, e
	}
	g, e := m.gateway(b)
	if e != nil {
		return Plan{}, e
	}
	return AdapterFor(b.AdapterID).Plan(b, g)
}
func (m *Manager) Apply(id string, revision int64, fp string) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result, err := m.apply(id, revision, fp)
	if err == nil {
		delete(m.last, id)
	}
	return result, err
}
func (m *Manager) apply(id string, revision int64, fp string) (Result, error) {
	b, e := m.Store.Binding(id)
	if e != nil {
		return Result{}, e
	}
	if b.Revision != revision {
		return Result{}, state.ErrConflict
	}
	if !b.Enabled {
		return Result{}, errors.New("binding is disabled")
	}
	g, e := m.gateway(b)
	if e != nil {
		return Result{}, e
	}
	if !g.Ready {
		return Result{}, errors.New("gateway is not ready")
	}
	current, e := m.Engine.Status(id)
	if e != nil {
		return Result{}, e
	}
	if current.Status == "applied" {
		_, e = AdapterFor(b.AdapterID).Verify(b, g)
		return current, withDiagnostic(e, "verify_existing", "", "")
	}
	if current.Status != "unmanaged" && current.Status != "restored" {
		reason, field := "unfinished_journal", ""
		if len(current.Conflicts) > 0 {
			reason, field = "managed_field_mismatch", diagnosticField(current.Conflicts[0])
			if len(current.Conflicts[0]) > 0 && current.Conflicts[0][0] == "catalog" {
				reason = "catalog_mismatch"
			}
		} else if current.Status == "conflict" {
			reason = "recovery_conflict"
		}
		return current, withDiagnostic(ErrChanged, "apply_precondition", reason, field)
	}
	p, e := AdapterFor(b.AdapterID).Plan(b, g)
	if e != nil {
		return Result{}, withDiagnostic(e, "plan", "", "")
	}
	if fp != "" && fp != p.Fingerprint {
		return Result{}, withDiagnostic(ErrChanged, "plan_check", "plan_changed", "")
	}
	result, e := m.Engine.Apply(p)
	if e == nil {
		_, e = AdapterFor(b.AdapterID).Verify(b, g)
		e = withDiagnostic(e, "verify_written", "", "")
	}
	return result, e
}
func (m *Manager) Restore(id string, revision int64) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping {
		return Result{}, errors.New("management is stopping")
	}
	b, e := m.Store.Binding(id)
	if e != nil {
		return Result{}, e
	}
	if b.Revision != revision {
		return Result{}, state.ErrConflict
	}
	result, err := m.Engine.Recover(id)
	if err == nil {
		delete(m.last, id)
	}
	return result, err
}
func (m *Manager) SetEnabled(id string, revision int64, enabled bool) (state.Binding, Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping {
		return state.Binding{}, Result{}, errors.New("management is stopping")
	}
	b, e := m.Store.Binding(id)
	if e != nil {
		return b, Result{}, e
	}
	if b.Revision != revision {
		return b, Result{}, state.ErrConflict
	}
	result := Result{BindingID: id, Status: "pending"}
	if !enabled {
		result, e = m.Engine.Recover(id)
		if e != nil {
			return b, result, e
		}
	}
	b, e = m.Store.EnableBinding(id, revision, enabled)
	if enabled && errors.Is(e, state.ErrNotFound) {
		// The binding was found above; activation now requires its default model.
		e = fmt.Errorf("%w: %w", ErrModelUnavailable, e)
	}
	if e == nil {
		delete(m.last, id)
	}
	return b, result, e
}
func (m *Manager) Status() ([]Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	bs, e := m.Store.Bindings()
	if e != nil {
		return nil, e
	}
	out := []Result{}
	for _, b := range bs {
		r, e := m.Engine.Status(b.ID)
		if e != nil {
			r = Result{BindingID: b.ID, Status: "recovery_pending"}
		}
		if b.Enabled && r.Status == "unmanaged" {
			r.Status = "pending"
		}
		if previous, ok := m.last[b.ID]; ok && (previous.Status == "apply_failed" || previous.Status == "needs_sync" || previous.Status == "recovery_pending") && (r.Status == "pending" || r.Status == "restored" || r.Status == "applied") {
			r.Status = previous.Status
		}
		out = append(out, r)
	}
	return out, nil
}

// Start never silently captures an old written value as a new original.
func (m *Manager) Start() ([]Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.last == nil {
		m.last = map[string]Result{}
	}
	m.ready = true
	m.stopping = false
	bs, e := m.Store.Bindings()
	if e != nil {
		return nil, e
	}
	out := []Result{}
	for _, b := range bs {
		if !b.Enabled {
			continue
		}
		status, e := m.Engine.Status(b.ID)
		if e != nil {
			status = Result{BindingID: b.ID, OperationID: status.OperationID, Status: "recovery_pending"}
		} else {
			if status.Status == "prepared" {
				status, e = m.Engine.Recover(b.ID)
				if e != nil {
					status.Status = "recovery_pending"
				}
			} else if status.Status == "applied" {
				// Validate current support and desired metadata before touching a
				// healthy journal. Unsupported external changes remain repairable.
				var g GatewayBinding
				_, e = AdapterFor(b.AdapterID).Inspect(b)
				if e == nil {
					g, e = m.gateway(b)
				}
				if e == nil && b.AdapterID == "codex" && len(g.Models) > 0 && (b.ModelScope == "all-mapped" || g.CatalogVersion != "") {
					_, _, e = buildCatalogTemplates(g.Models, g.CatalogVersion, g.CatalogTemplates)
				}
				if e == nil {
					if _, stale := AdapterFor(b.AdapterID).Verify(b, g); stale != nil {
						status, e = m.Engine.Reconcile(b, g)
					}
				}
			}
			if e == nil && status.Status == "conflict" {
				e = ErrChanged
			}
			if e == nil {
				status, e = m.apply(b.ID, b.Revision, "")
			}
			if e != nil && (status.Status == "" || status.Status == "pending" || status.Status == "applied" || status.Status == "restored") {
				status = Result{BindingID: b.ID, OperationID: status.OperationID, Status: "apply_failed"}
			}
		}
		m.last[b.ID] = status
		out = append(out, status)
	}
	return out, nil
}

// PreserveForRestart gates further management without restoring client files.
// The approved binding, token and original recovery journal remain unchanged so
// the replacement core can verify/reuse them. Ordinary Stop still restores.
func (m *Manager) PreserveForRestart() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopping = true
	m.ready = false
}

func (m *Manager) Stop() ([]Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopping = true
	m.ready = false
	bs, e := m.Store.Bindings()
	if e != nil {
		return nil, e
	}
	out := []Result{}
	for _, b := range bs {
		r, e := m.Engine.Recover(b.ID)
		if e != nil {
			r = Result{BindingID: b.ID, Status: "recovery_pending"}
		}
		out = append(out, r)
	}
	return out, nil
}

func (m *Manager) SetModels(id string, revision int64, model, scope string) (state.Binding, Result, error) {
	if scope != "single" && scope != "all-compatible" && scope != "all-mapped" {
		return state.Binding{}, Result{}, errors.New("model_scope must explicitly be single, all-mapped or all-compatible")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopping {
		return state.Binding{}, Result{}, errors.New("management is stopping")
	}
	b, err := m.Store.Binding(id)
	if err != nil {
		return b, Result{}, err
	}
	if b.Revision != revision {
		return b, Result{}, state.ErrConflict
	}
	b.Model = model
	b.ModelScope = scope
	if len(b.CodexCatalog) == 0 {
		b.CodexVersion = ""
	}
	configured, err := m.Store.Model(model)
	if err != nil {
		return b, Result{}, err
	}
	if b.AdapterID == "codex" && scope != "all-mapped" && len(b.CodexCatalog) == 0 && configured.Codex != nil {
		b.CodexVersion = configured.Codex.Version
	}
	if err = b.Validate(); err != nil {
		return b, Result{}, err
	}
	g, err := m.gateway(b)
	if err != nil {
		return b, Result{}, err
	}
	if b.AdapterID == "codex" && b.ModelScope != "all-mapped" && (!g.Tools || !g.Streaming) {
		return b, Result{}, errors.New("Codex model requires tools and streaming")
	}
	if len(g.Models) > 0 && g.CatalogVersion != "" {
		if _, _, err = buildCatalogTemplates(g.Models, g.CatalogVersion, g.CatalogTemplates); err != nil {
			return b, Result{}, err
		}
	}
	restored, err := m.Engine.Recover(id)
	if err != nil {
		return b, restored, err
	}
	if restored.Status == "conflict" {
		return b, restored, ErrChanged
	}
	b, err = m.Store.SetBindingModels(id, revision, model, scope, b.CodexVersion)
	if err == nil {
		delete(m.last, id)
	}
	return b, restored, err
}
