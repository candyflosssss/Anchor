package core

import (
	"errors"
	"net/http"

	"github.com/candyflosssss/Anchor/internal/apierror"
	"github.com/candyflosssss/Anchor/internal/skills"
)

func skillError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, skills.ErrUnsafe):
		fail(w, apierror.SkillUnsafe)
	case errors.Is(err, skills.ErrChanged):
		fail(w, apierror.SkillChanged)
	case errors.Is(err, skills.ErrLimit):
		fail(w, apierror.SkillLimit)
	case errors.Is(err, skills.ErrLink):
		fail(w, apierror.SkillLinkUnavailable)
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
