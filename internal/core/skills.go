package core

import (
	"errors"
	"net/http"

	"github.com/candyflosssss/Anchor/internal/skills"
)

func skillError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, skills.ErrUnsafe):
		fail(w, 409, "skill_unsafe", "skill path, permissions or file types are unsupported")
	case errors.Is(err, skills.ErrChanged):
		fail(w, 409, "skill_changed", "skill content or managed link changed; refresh before retrying")
	case errors.Is(err, skills.ErrLimit):
		fail(w, 400, "skill_limit", "skill inventory or content exceeds the documented limits")
	case errors.Is(err, skills.ErrLink):
		fail(w, 409, "skill_link_unavailable", "symbolic link creation failed; no copy fallback was performed")
	default:
		apiError(w, err)
	}
}
func (s *Server) skillRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /control/v1/skills", func(w http.ResponseWriter, r *http.Request) {
		env, err := s.localAgentEnvironment()
		if err != nil {
			skillError(w, err)
			return
		}
		data, err := s.Skills.Workspace(env)
		if err != nil {
			skillError(w, err)
			return
		}
		writeJSON(w, 200, data)
	})
	mux.HandleFunc("POST /control/v1/skills/detail", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in skills.SkillQuery
		if !decode(w, r, &in) {
			return
		}
		env, err := s.localAgentEnvironment()
		if err != nil {
			skillError(w, err)
			return
		}
		data, err := s.Skills.Detail(env, in)
		if err != nil {
			skillError(w, err)
			return
		}
		writeJSON(w, 200, data)
	})
	mux.HandleFunc("POST /control/v1/skills/action", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in skills.SkillAction
		if !decode(w, r, &in) {
			return
		}
		env, err := s.localAgentEnvironment()
		if err != nil {
			skillError(w, err)
			return
		}
		data, err := s.Skills.Action(env, in)
		if err != nil {
			skillError(w, err)
			return
		}
		writeJSON(w, 200, data)
	})
}
