package core

import (
	"github.com/candyflosssss/Anchor/internal/state"
	"net/http"
)

func (s *Server) codexSettingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /control/v1/settings/codex", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		out, err := s.Agents.CodexSettings()
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /control/v1/settings/codex", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in state.SaveCodexSettings
		if !decode(w, r, &in) {
			return
		}
		out, err := s.Agents.SaveCodexSettings(in)
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
}
