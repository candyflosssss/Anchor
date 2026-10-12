package core

import (
	"errors"
	"net/http"

	"github.com/candyflosssss/Anchor/internal/managed"
	"github.com/candyflosssss/Anchor/internal/state"
)

func agentError(w http.ResponseWriter, e error) {
	if errors.Is(e, state.ErrConflict) || errors.Is(e, state.ErrNotFound) {
		apiError(w, e)
		return
	}
	// A config parser or filesystem error may contain private values or paths.
	fail(w, 409, "agent_operation_failed", "unsafe, changed, unsupported or unavailable target; inspect status and recovery records locally")
}

// Local catalog snapshots can be larger than the client response budget.
// Public binding responses retain policy choices, never full instruction entries.
func publicBinding(b state.Binding) state.Binding {
	b.CodexCatalog = nil
	return b
}

func (s *Server) agentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /control/v1/agents/workspace", func(w http.ResponseWriter, r *http.Request) {
		env, e := s.localAgentEnvironment()
		if e != nil {
			agentError(w, e)
			return
		}
		v, e := s.Agents.Workspace(env)
		if e != nil {
			agentError(w, e)
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
			agentError(w, e)
			return
		}
		if _, e = s.Agents.Enroll(env, in); e != nil {
			agentError(w, e)
			return
		}
		v, e := s.Agents.Workspace(env)
		if e != nil {
			agentError(w, e)
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
			fail(w, 400, "approval_required", "explicit approved configuration root required")
			return
		}
		b, e := s.Agents.Add(in.Binding)
		if e != nil {
			agentError(w, e)
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
		v, e := (managed.Codex{}).Inspect(b)
		if e != nil {
			agentError(w, e)
			return
		}
		status, e := s.Agents.Engine.Status(b.ID)
		if e != nil {
			status = managed.Result{BindingID: b.ID, Status: "recovery_pending"}
		}
		writeJSON(w, 200, map[string]any{"binding": publicBinding(b), "inspection": v, "management": status})
	})
	mux.HandleFunc("GET /control/v1/agents/{id}/plan", func(w http.ResponseWriter, r *http.Request) {
		p, e := s.Agents.Plan(r.PathValue("id"))
		if e != nil {
			agentError(w, e)
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
				agentError(w, e)
				return
			}
			writeJSON(w, 200, v)
		case "set-models":
			if !state.ValidID(in.Model) || (in.ModelScope != "single" && in.ModelScope != "all-compatible") {
				fail(w, 400, "invalid_model_policy", "model and explicit single or all-compatible model_scope required")
				return
			}
			b, v, e := s.Agents.SetModels(id, in.ExpectedRevision, in.Model, in.ModelScope)
			if e != nil {
				agentError(w, e)
				return
			}
			writeJSON(w, 200, map[string]any{"binding": publicBinding(b), "management": v, "apply_required": true})
		case "apply":
			if in.Fingerprint == "" {
				fail(w, 400, "plan_required", "apply requires a reviewed plan fingerprint")
				return
			}
			v, e := s.Agents.Apply(id, in.ExpectedRevision, in.Fingerprint)
			if e != nil {
				agentError(w, e)
				return
			}
			writeJSON(w, 200, v)
		case "restore":
			v, e := s.Agents.Restore(id, in.ExpectedRevision)
			if e != nil {
				agentError(w, e)
				return
			}
			writeJSON(w, 200, v)
		case "enable", "disable":
			b, v, e := s.Agents.SetEnabled(id, in.ExpectedRevision, action == "enable")
			if e != nil {
				agentError(w, e)
				return
			}
			writeJSON(w, 200, map[string]any{"binding": publicBinding(b), "management": v})
		default:
			fail(w, 404, "not_found", "unknown agent action")
		}
	})
}

func (s *Server) localAgentEnvironment() (managed.Environment, error) {
	if s.AgentEnvironment != nil {
		return s.AgentEnvironment()
	}
	return managed.LocalEnvironment()
}
