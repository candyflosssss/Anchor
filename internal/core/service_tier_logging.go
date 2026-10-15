package core

import (
	"encoding/json"
	"net/http"

	"github.com/candyflosssss/Anchor/internal/protocol"
)

func requestedServiceTier(raw map[string]json.RawMessage) string {
	value, exists := raw["service_tier"]
	if !exists {
		return "unspecified"
	}
	var tier any
	if json.Unmarshal(value, &tier) != nil {
		return "unknown"
	}
	return protocol.SafeServiceTier(tier)
}

func observeRequestedServiceTier(r *http.Request, raw map[string]json.RawMessage) {
	if t := trace(r); t != nil {
		// Keep ingress metadata when the protocol adapter replaces the body.
		if _, exists := t.row.Evidence["requested_service_tier"]; !exists {
			t.row.Evidence["requested_service_tier"] = requestedServiceTier(raw)
		}
	}
}
