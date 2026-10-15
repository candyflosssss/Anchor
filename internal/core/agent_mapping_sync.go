package core

import "net/http"

// Call immediately after a committed management mutation, before building its
// response. Even if response construction subsequently fails, client model
// lists must reflect the committed mapping. A synchronization conflict must
// not turn a committed mutation into a retryable write failure.
func (s *Server) syncAgentMappings(w http.ResponseWriter) {
	results, err := s.Agents.SyncMappings()
	partial := err != nil
	for _, result := range results {
		if result.Status != "applied" {
			partial = true
			// Do not log private configuration or credentials from an adapter error.
			s.Log.Warn("agent model mapping sync incomplete", "binding_id", result.BindingID, "status", result.Status)
		}
	}
	if partial {
		w.Header().Set("X-Anchor-Agent-Sync", "needs_sync")
	} else {
		w.Header().Set("X-Anchor-Agent-Sync", "applied")
	}
}
