package core

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/candyflosssss/Anchor/internal/state"
)

type forwardAttemptResult struct {
	attempted bool
	success   bool
	status    int
	after     string
}

func (s *Server) forward(w http.ResponseWriter, r *http.Request, request map[string]json.RawMessage, public string, c state.Candidate, p state.Provider, key state.ProviderKey, stream bool, routeModel state.Model, affinity string) bool {
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(p.TimeoutSeconds)*time.Second)
	defer cancel()
	var result forwardAttemptResult
	defer func() {
		if !result.attempted || r.Context().Err() != nil {
			return
		}
		s.finishKey(p, key.ID, result.status, result.after, result.success)
	}()
	// Codex owns retries. Each provider/key/model is attempted once per request;
	// the outer routing loop still permits safe key/candidate failover.
	return s.forwardAttempt(w, r, request, public, c, p, key, stream, routeModel, affinity, ctx, &result)
}
