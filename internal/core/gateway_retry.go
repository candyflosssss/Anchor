package core

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/candyflosssss/Anchor/internal/apierror"
	"github.com/candyflosssss/Anchor/internal/protocol"
	"github.com/candyflosssss/Anchor/internal/state"
)

type forwardAttemptResult struct {
	attempted bool
	success   bool
	status    int
	after     string
	opaque    *protocol.Recovery
}

func (s *Server) forward(w http.ResponseWriter, r *http.Request, translation *protocol.Session, public string, c state.Candidate, p state.Provider, key state.ProviderKey, stream bool, routeModel state.Model, affinity string) bool {
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(p.TimeoutSeconds)*time.Second)
	defer cancel()
	firstSeconds := p.FirstTokenTimeoutSeconds
	if firstSeconds == 0 {
		firstSeconds = state.DefaultFirstTokenTimeoutSeconds
	}
	ctx, first := withFirstOutputTimeout(ctx, time.Duration(firstSeconds)*time.Second)
	defer first.close()
	var result forwardAttemptResult
	defer func() {
		if !result.attempted || r.Context().Err() != nil {
			return
		}
		s.finishKey(p, key.ID, result.status, result.after, result.success)
	}()
	// Ordinary retries belong to the caller. A definite rejection of historical
	// reasoning permits one changed request, before any downstream output.
	base := s.pickURL(p)
	allowRecovery := translation.CanRecover(key.EffectiveProtocol(c.Model))
	if t := trace(r); t != nil && t.encryptedContextRetried {
		allowRecovery = false
	}
	retry := s.forwardAttempt(w, r, translation, public, c, p, key, stream, routeModel, affinity, base, allowRecovery, ctx, first, &result)
	if result.opaque == nil {
		return retry
	}
	if ctx.Err() != nil {
		if r.Context().Err() == nil {
			fail(w, apierror.RecoveryTimeout)
		}
		return false
	}
	recovery := result.opaque
	if t := trace(r); t != nil {
		t.encryptedContextRetried = true
		t.row.Evidence["encrypted_context_recovery"] = "pending"
		t.row.Evidence["encrypted_context_reason"] = recovery.Reason
		t.row.Evidence["encrypted_context_removed_reasoning"] = strconv.Itoa(recovery.Removed)
		t.row.Evidence["encrypted_context_retry_count"] = "1"
		if s.logs != nil {
			s.logs.Request(t.row)
		}
	}
	retry = s.forwardAttempt(w, r, recovery.Session(), public, c, p, key, stream, routeModel, affinity, base, false, ctx, first, &result)
	if t := trace(r); t != nil {
		outcome := "failed"
		if result.success {
			outcome = "succeeded"
		} else if r.Context().Err() != nil {
			outcome = "cancelled"
		}
		t.row.Evidence["encrypted_context_recovery"] = outcome
	}
	return retry
}
