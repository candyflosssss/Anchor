package core

import (
	"errors"
	"net/http"

	"github.com/candyflosssss/Anchor/internal/modeldev"
	"github.com/candyflosssss/Anchor/internal/state"
)

func (s *Server) modelCatalogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /control/v1/model-catalog/query", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in modeldev.ModelCatalogQuery
		if !decode(w, r, &in) {
			return
		}
		if in.Validate() != nil {
			apiError(w, state.ErrInvalid)
			return
		}
		out, err := s.modelCatalog.Query(r.Context(), in)
		if err != nil {
			modelCatalogError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /control/v1/model-catalog/estimate", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		var in modeldev.CatalogEstimateInput
		if !decode(w, r, &in) {
			return
		}
		if in.Validate() != nil {
			apiError(w, state.ErrInvalid)
			return
		}
		out, err := s.modelCatalog.Estimate(r.Context(), in)
		if err != nil {
			modelCatalogError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
}
func modelCatalogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, modeldev.ErrInvalid):
		apiError(w, state.ErrInvalid)
	case errors.Is(err, modeldev.ErrNotFound):
		apiError(w, state.ErrNotFound)
	default:
		fail(w, http.StatusBadGateway, "catalog_unavailable", "model reference catalog unavailable")
	}
}
