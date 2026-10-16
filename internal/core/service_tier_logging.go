package core

import (
	"encoding/json"
	"net/http"
	"strings"

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
			recordRawServiceTier(t.row.Evidence, "requested_service_tier_raw", raw["service_tier"], strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), r.Header.Get("x-api-key"))
		}
	}
}

// sentServiceTier observes the serialized, protocol-converted dispatch body,
// after any fixed mapping policy has replaced the caller's tier.
func sentServiceTier(body []byte) string {
	var fields struct {
		Tier json.RawMessage `json:"service_tier"`
	}
	if json.Unmarshal(body, &fields) != nil {
		return "unknown"
	}
	if len(fields.Tier) == 0 {
		return "unspecified"
	}
	return requestedServiceTier(map[string]json.RawMessage{"service_tier": fields.Tier})
}

// Fast includes Ultrafast. Missing/automatic metadata is not a negative result.
func fastTriggered(tier string) string {
	switch tier {
	case "priority", "fast", "ultrafast":
		return "true"
	case "default", "flex", "scale":
		return "false"
	default:
		return "unknown"
	}
}

func recordReturnedServiceTier(evidence map[string]string, tier string, raw string, secrets ...string) {
	recordRawServiceTier(evidence, "upstream_service_tier_raw", json.RawMessage(raw), secrets...)
	evidence["fast_triggered"] = fastTriggered(tier)
	if tier != "" {
		evidence["upstream_service_tier"] = tier
	}
}

// Only the service_tier field is inspected; request/response bodies are never logged.
func recordRawServiceTier(evidence map[string]string, name string, raw json.RawMessage, secrets ...string) {
	delete(evidence, name)
	if safe := protocol.SanitizeRawServiceTier(raw, secrets...); safe != "" {
		evidence[name] = safe
	}
}

func recordSentServiceTier(evidence map[string]string, body []byte, secrets ...string) {
	evidence["sent_service_tier"] = sentServiceTier(body)
	var fields struct {
		Tier json.RawMessage `json:"service_tier"`
	}
	if json.Unmarshal(body, &fields) == nil {
		recordRawServiceTier(evidence, "sent_service_tier_raw", fields.Tier, secrets...)
	}
}
