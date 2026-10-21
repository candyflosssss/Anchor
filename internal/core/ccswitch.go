package core

import (
	"net/http"

	"github.com/candyflosssss/Anchor/internal/ccswitch"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (s *Server) ccSwitchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /control/v1/import/cc-switch", func(w http.ResponseWriter, r *http.Request) {
		// Both preview and apply read source credentials; owner identity and
		// instance pinning are required even though reports are redacted.
		if !s.identity(w, r) {
			return
		}
		var in ccswitch.Request
		if !decode(w, r, &in) {
			return
		}
		if (!in.Approved && in.Fingerprint != "") || (in.Approved && in.Fingerprint == "") {
			apiError(w, state.ErrInvalid)
			return
		}
		plan, err := ccswitch.Build(r.Context(), in.DatabasePath, s.Store)
		if err != nil {
			apiError(w, err)
			return
		}
		if in.Approved {
			if plan.Report.Failure != "" || plan.Report.Fingerprint != in.Fingerprint {
				apiError(w, state.ErrConflict)
				return
			}
			if err = s.Store.ImportProviders(r.Context(), plan.Providers, plan.Models); err != nil {
				apiError(w, err)
				return
			}
			plan.Report.Applied = true
			plan.Report.ImportedProviders = len(plan.Providers)
			for _, p := range plan.Providers {
				plan.Report.ImportedKeys += len(p.Keys)
			}
			plan.Report.ImportedModels = len(plan.Models)
			s.syncAgentMappings(w)
		}
		writeJSON(w, http.StatusOK, plan.Report)
	})
}
