package core

import (
	"errors"
	"net/http"

	"github.com/candyflosssss/Anchor/internal/apierror"
	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/state"
)

func agentError(w http.ResponseWriter, e error) {
	// Only allowlisted categories cross the boundary. Never return a parser or
	// filesystem error string, which may contain private configuration values.
	id := apierror.AgentOperationFailed
	switch {
	case errors.Is(e, managed.ErrModelUnavailable):
		id = apierror.AgentModelUnavailable
	case errors.Is(e, managed.ErrCatalogInvalid):
		id = apierror.AgentCatalogInvalid
	case errors.Is(e, managed.ErrChanged):
		id = apierror.AgentConfigConflict
	case errors.Is(e, managed.ErrUnsafe):
		id = apierror.AgentConfigUnsafe
	case errors.Is(e, state.ErrConflict), errors.Is(e, state.ErrNotFound):
		apiError(w, e)
		return
	}
	fail(w, id)
}

// logAgentFailure records safe diagnostic labels, never the underlying error.
// Binding metadata is loaded from validated state, not copied from request input.
func (s *Server) logAgentFailure(action, bindingID string, err error) {
	if s.Log == nil {
		return
	}
	d := managed.Diagnose(err)
	attrs := []any{"action", action, "stage", d.Stage, "reason", d.Reason}
	if d.Field != "" {
		attrs = append(attrs, "field", d.Field)
	}
	if s.Store != nil && bindingID != "" {
		if b, e := s.Store.Binding(bindingID); e == nil {
			attrs = append(attrs, "binding_id", b.ID, "adapter_id", b.AdapterID)
		}
	}
	s.Log.Warn("agent operation failed", attrs...)
}

func (s *Server) agentError(w http.ResponseWriter, action, bindingID string, err error) {
	s.logAgentFailure(action, bindingID, err)
	agentError(w, err)
}

// Local catalog snapshots can be larger than the client response budget.
// Public binding responses retain policy choices, never full instruction entries.
func publicBinding(b state.Binding) state.Binding {
	b.CodexCatalog = nil
	return b
}

func (s *Server) agentRoutes(mux *http.ServeMux) {
	s.codexSettingsRoutes(mux)
	mux.HandleFunc("GET /control/v1/agents/workspace", func(w http.ResponseWriter, r *http.Request) {
		env, e := s.localAgentEnvironment()
		if e != nil {
			s.agentError(w, "workspace", "", e)
			return
		}
		v, e := s.Agents.Workspace(env)
		if e != nil {
			s.agentError(w, "workspace", "", e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /control/v1/agents/enroll", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in managed.EnrollAgent
		if !decode(w, r, &in) {
			return
		}
		env, e := s.localAgentEnvironment()
		if e != nil {
			s.agentError(w, "workspace", "", e)
			return
		}
		if b, err := s.Agents.Enroll(env, in); err != nil {
			s.agentError(w, "enroll", b.ID, err)
			return
		}
		v, e := s.Agents.Workspace(env)
		if e != nil {
			s.agentError(w, "workspace", "", e)
			return
		}
		writeJSON(w, 200, v)
	})

	mux.HandleFunc("GET /control/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		b, e := s.Store.Bindings()
		if e != nil {
			apiError(w, e)
			return
		}
		for i := range b {
			b[i] = publicBinding(b[i])
		}
		writeJSON(w, 200, b)
	})
	mux.HandleFunc("POST /control/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in struct {
			Binding  state.Binding `json:"binding"`
			Approved bool          `json:"approved"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !in.Approved {
			fail(w, apierror.ApprovalRequired)
			return
		}
		b, e := s.Agents.Add(in.Binding)
		if e != nil {
			s.agentError(w, "add", b.ID, e)
			return
		}
		writeJSON(w, 200, publicBinding(b))
	})
	mux.HandleFunc("GET /control/v1/agents/{id}/inspect", func(w http.ResponseWriter, r *http.Request) {
		b, e := s.Store.Binding(r.PathValue("id"))
		if e != nil {
			apiError(w, e)
			return
		}
		v, e := managed.AdapterFor(b.AdapterID).Inspect(b)
		if e != nil {
			s.agentError(w, "inspect", b.ID, e)
			return
		}
		status, e := s.Agents.Engine.Status(b.ID)
		if e != nil {
			s.logAgentFailure("inspect-status", b.ID, e)
			status = managed.Result{BindingID: b.ID, Status: "recovery_pending"}
		}
		writeJSON(w, 200, map[string]any{"binding": publicBinding(b), "inspection": v, "management": status})
	})
	mux.HandleFunc("GET /control/v1/agents/{id}/plan", func(w http.ResponseWriter, r *http.Request) {
		p, e := s.Agents.Plan(r.PathValue("id"))
		if e != nil {
			s.agentError(w, "plan", r.PathValue("id"), e)
			return
		}
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("POST /control/v1/agents/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in struct {
			ExpectedRevision int64  `json:"expected_revision"`
			Fingerprint      string `json:"fingerprint,omitempty"`
			Model            string `json:"model"`
			ModelScope       string `json:"model_scope"`
		}
		if !decode(w, r, &in) {
			return
		}
		id := r.PathValue("id")
		action := r.PathValue("action")
		switch action {
		case "sync":
			v, e := s.Agents.Refresh(id, in.ExpectedRevision)
			if e != nil {
				s.agentError(w, action, id, e)
				return
			}
			writeJSON(w, 200, v)
		case "set-models":
			if !state.ValidModelID(in.Model) || (in.ModelScope != "single" && in.ModelScope != "all-compatible" && in.ModelScope != "all-mapped") {
				fail(w, apierror.InvalidModelPolicy)
				return
			}
			b, v, e := s.Agents.SetModels(id, in.ExpectedRevision, in.Model, in.ModelScope)
			if e != nil {
				s.agentError(w, action, id, e)
				return
			}
			writeJSON(w, 200, map[string]any{"binding": publicBinding(b), "management": v, "apply_required": true})
		case "apply":
			if in.Fingerprint == "" {
				fail(w, apierror.PlanRequired)
				return
			}
			v, e := s.Agents.Apply(id, in.ExpectedRevision, in.Fingerprint)
			if e != nil {
				s.agentError(w, action, id, e)
				return
			}
			writeJSON(w, 200, v)
		case "restore":
			v, e := s.Agents.Restore(id, in.ExpectedRevision)
			if e != nil {
				s.agentError(w, action, id, e)
				return
			}
			writeJSON(w, 200, v)
		case "enable", "disable":
			b, v, e := s.Agents.SetEnabled(id, in.ExpectedRevision, action == "enable")
			if e != nil {
				s.agentError(w, action, id, e)
				return
			}
			writeJSON(w, 200, map[string]any{"binding": publicBinding(b), "management": v})
		default:
			fail(w, apierror.UnknownAgentAction)
		}
	})
}

func (s *Server) localAgentEnvironment() (managed.Environment, error) {
	if s.AgentEnvironment != nil {
		return s.AgentEnvironment()
	}
	return managed.LocalEnvironment()
}
