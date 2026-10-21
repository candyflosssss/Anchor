package core

import (
	"github.com/candyflosssss/Anchor/internal/state"
	"net/http"
	"time"
)

func (s *Server) keyCooldownRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /control/v1/settings/key-cooldown", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		out, err := s.Store.KeyCooldownSettings()
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /control/v1/settings/key-cooldown", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in state.SaveKeyCooldownSettings
		if !decode(w, r, &in) {
			return
		}
		out, err := s.saveKeyCooldownSettings(in)
		if err != nil {
			apiError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
}
func (s *Server) saveKeyCooldownSettings(in state.SaveKeyCooldownSettings) (state.KeyCooldownSettings, error) {
	// Serialize policy changes with in-flight results and retain the last valid policy.
	s.mu.Lock()
	defer s.mu.Unlock()
	out, err := s.Store.SaveKeyCooldownSettings(in)
	if err != nil {
		return out, err
	}
	s.keyPolicy = out
	s.routing = map[string]*modelSchedule{}
	for _, h := range s.health {
		h.Failures = 0
		h.Score = 0
		h.errors = nil
		h.Until = time.Time{}
	}
	return out, nil
}
