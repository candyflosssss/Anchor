package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/candyflosssss/Anchor/internal/logbook"
)

func logError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, logbook.ErrInvalid):
		fail(w, 400, "invalid_log_query", "invalid log query")
	case errors.Is(err, logbook.ErrNotFound):
		fail(w, 404, "log_not_found", "log not found")
	default:
		fail(w, 503, "log_storage_unavailable", "log storage unavailable")
	}
}
func (s *Server) logRoutes(m *http.ServeMux) {
	ready := func(w http.ResponseWriter) bool {
		if s.logs == nil {
			logError(w, logbook.ErrUnavailable)
			return false
		}
		return true
	}
	m.HandleFunc("POST /control/v1/logs/suggestions", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) || !ready(w) {
			return
		}
		var q logbook.SuggestionsQuery
		if !decode(w, r, &q) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		v, err := s.logs.Suggest(ctx, q)
		if err != nil {
			logError(w, err)
			return
		}
		writeJSON(w, 200, v)
	})
	m.HandleFunc("POST /control/v1/logs/stats", func(w http.ResponseWriter, r *http.Request) {
		if !s.identity(w, r) {
			return
		}
		if !ready(w) {
			return
		}
		var q logbook.StatsQuery
		if !decode(w, r, &q) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		v, err := s.logs.Statistics(ctx, q)
		if err != nil {
			logError(w, err)
			return
		}
		writeJSON(w, 200, v)
	})
	m.HandleFunc("POST /control/v1/logs/query", func(w http.ResponseWriter, r *http.Request) {
		if !ready(w) {
			return
		}
		var q logbook.Query
		if !decode(w, r, &q) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		p, err := s.logs.List(ctx, q)
		if err != nil {
			logError(w, err)
			return
		}
		// Retain compact service-tier and recovery evidence; detail/export retain all diagnostics.
		for i := range p.Requests {
			p.Requests[i].Attempts = []logbook.Attempt{}
			evidence := map[string]string{}
			for _, name := range []string{"sent_service_tier", "sent_service_tier_raw", "fast_triggered", "upstream_service_tier", "upstream_service_tier_raw", "encrypted_context_recovery", "encrypted_context_reason", "encrypted_context_removed_reasoning", "encrypted_context_retry_count"} {
				if value, ok := p.Requests[i].Evidence[name]; ok {
					evidence[name] = value
				}
			}
			p.Requests[i].Evidence = evidence
		}
		writeJSON(w, 200, p)
	})
	m.HandleFunc("GET /control/v1/logs/requests/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !ready(w) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		v, err := s.logs.Detail(ctx, r.PathValue("id"))
		if err != nil {
			logError(w, err)
			return
		}
		writeJSON(w, 200, v)
	})
	m.HandleFunc("GET /control/v1/logs/settings", func(w http.ResponseWriter, r *http.Request) {
		if ready(w) {
			writeJSON(w, 200, s.logs.Settings())
		}
	})
	m.HandleFunc("PUT /control/v1/logs/settings", func(w http.ResponseWriter, r *http.Request) {
		if !ready(w) {
			return
		}
		var v logbook.Settings
		if !decode(w, r, &v) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := s.logs.Configure(ctx, v); err != nil {
			logError(w, err)
			return
		}
		writeJSON(w, 200, s.logs.Settings())
	})
	m.HandleFunc("POST /control/v1/logs/export", func(w http.ResponseWriter, r *http.Request) {
		if !ready(w) {
			return
		}
		var q logbook.Query
		if !decode(w, r, &q) {
			return
		}
		if q.Validate() != nil {
			logError(w, logbook.ErrInvalid)
			return
		}
		q.Limit = 100
		q.Cursor = 0
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		p, err := s.logs.List(ctx, q)
		if err != nil {
			logError(w, err)
			return
		}
		q.Until = p.Latest
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Cache-Control", "no-store")
		enc := json.NewEncoder(w)
		for count := 0; count < 200000; {
			if q.Kind == "requests" {
				for _, v := range p.Requests {
					if enc.Encode(v) != nil {
						return
					}
					count++
				}
			} else {
				for _, v := range p.Events {
					if enc.Encode(v) != nil {
						return
					}
					count++
				}
			}
			if !p.HasMore {
				return
			}
			q.Cursor = p.NextCursor
			p, err = s.logs.List(ctx, q)
			if err != nil {
				panic(http.ErrAbortHandler)
			}
		}
	})
}
