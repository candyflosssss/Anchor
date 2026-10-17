package core

import (
	"net/http"

	"github.com/candyflosssss/Anchor/internal/egress"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (s *Server) proxyWorkspace() (state.ProxyWorkspace, error) {
	w, e := s.Store.ProxyWorkspace()
	if e != nil {
		return w, e
	}
	w.Engine = s.egress.Status()
	return w, nil
}
func (s *Server) proxyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /control/v1/proxies", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.proxyWorkspace()
		if e != nil {
			apiError(w, e)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /control/v1/proxies/action", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in state.ProxyAction
		if !decode(w, r, &in) {
			return
		}
		if e := s.Store.MutateProxy(in, egress.Normalize); e != nil {
			apiError(w, e)
			return
		}
		out, e := s.proxyWorkspace()
		if e != nil {
			apiError(w, e)
			return
		}
		s.egress.Reconcile(out.Profiles)
		out.Engine = s.egress.Status()
		writeJSON(w, 200, out)
	})
}
