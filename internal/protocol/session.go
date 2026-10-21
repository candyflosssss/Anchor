package protocol

import (
	"encoding/json"
)

// Session owns an immutable client baseline. It contains no credentials, routes,
// network resources or global state, and can prepare independent attempts concurrently.
// Native payloads are retained separately from the Responses-shaped policy view:
// that view is never a lossy intermediate for native dispatch.
type Session struct {
	client     string
	native     []byte
	baseline   []byte
	ingressErr error
}

// NewSession parses ingress without choosing an upstream. Cross-protocol validation
// is deferred until Prepare, while the gateway can enforce its existing policies.
func NewSession(raw []byte, client string) (*Session, error) {
	if !knownProtocol(client) {
		return nil, newTranslationError(client, "", StageRequest, "protocol", ErrUnsupported)
	}
	m, err := Decode(raw)
	if err != nil {
		return nil, newTranslationError(client, "", StageRequest, "invalid_json", err)
	}
	if client != "responses" {
		messages, ok := m["messages"].([]any)
		if !ok || len(messages) == 0 {
			return nil, newTranslationError(client, "", StageRequest, "request_content", ErrUnsupported)
		}
		for _, v := range messages {
			msg := obj(v)
			switch msg["role"] {
			case "user", "assistant", "system", "developer", "tool":
			default:
				return nil, newTranslationError(client, "", StageRequest, "request_content", ErrUnsupported)
			}
		}
	}
	s := &Session{client: client, native: append([]byte(nil), raw...)}
	if client == "responses" {
		s.baseline = append([]byte(nil), raw...)
		return s, nil
	}
	normalized, err := toResponses(m, client)
	if err != nil {
		// This is a policy projection, not a conversion. Unsupported native fields
		// stay in native and cause rejection if a different protocol is selected.
		s.ingressErr = err
		normalized = nativePolicyView(m, client)
	}
	s.baseline, _ = json.Marshal(normalized)
	return s, nil
}

func knownProtocol(p string) bool {
	return p == "responses" || p == "chat-completions" || p == "anthropic"
}
func (s *Session) ClientProtocol() string { return s.client }

// PolicyFields returns a fresh copy for access checks, affinity and accounting.
// Mutating it cannot change any dispatch or a subsequent candidate's baseline.
func (s *Session) PolicyFields() map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(s.baseline, &fields)
	return fields
}

func nativePolicyView(m Object, p string) Object {
	out := Object{}
	for _, k := range []string{"model", "stream", "store", "background", "previous_response_id", "conversation", "service_tier", "metadata", "user"} {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	if effort, ok := m["reasoning_effort"]; ok {
		out["reasoning"] = Object{"effort": effort}
	}
	// Only client-executed function declarations pass the existing gateway guard.
	if value, ok := m["tools"]; ok {
		tools, valid := value.([]any)
		if !valid {
			out["tools"] = value
			return out
		}
		projected := []any{}
		for _, v := range tools {
			t := obj(v)
			if p == "chat-completions" {
				if t["type"] != "function" || obj(t["function"]) == nil {
					projected = append(projected, Object{"type": "unsupported"})
					continue
				}
				projected = append(projected, Object{"type": "function", "name": obj(t["function"])["name"]})
			} else {
				if t == nil || t["type"] != nil {
					projected = append(projected, Object{"type": "unsupported"})
					continue
				}
				projected = append(projected, Object{"type": "function", "name": t["name"]})
			}
		}
		out["tools"] = projected
	}
	return out
}

type AttemptOptions struct {
	Protocol    string
	Model       string
	PublicModel string
	// ServiceTier is explicit routing policy, never inferred from a vendor/model name.
	ServiceTier string
}

// Attempt keeps its wire request and private return-path mapping together.
// It is owned by one upstream attempt; do not use one Attempt concurrently.
type Attempt struct {
	client, upstream, public string
	body                     []byte
	mapping                  *ChatMapping
	diagnostics              Diagnostics
}

func (s *Session) Prepare(o AttemptOptions) (*Attempt, error) {
	if !knownProtocol(o.Protocol) {
		return nil, newTranslationError(s.client, o.Protocol, StageRequest, "protocol", ErrUnsupported)
	}
	raw := s.baseline
	native := s.client == o.Protocol
	if native {
		raw = s.native
	} else if s.ingressErr != nil {
		return nil, newTranslationError(s.client, o.Protocol, StageRequest, ConversionReason(s.ingressErr), s.ingressErr)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	fields["model"], _ = json.Marshal(o.Model)
	if o.ServiceTier != "" {
		fields["service_tier"], _ = json.Marshal(o.ServiceTier)
	}
	if native && o.Protocol == "anthropic" {
		if _, exists := fields["max_tokens"]; !exists {
			fields["max_tokens"] = json.RawMessage("4096")
		}
	}
	if o.Protocol == "responses" {
		fields["store"] = json.RawMessage("false")
	}
	raw, _ = json.Marshal(fields)
	a := &Attempt{client: s.client, upstream: o.Protocol, public: o.PublicModel,
		diagnostics: Diagnostics{Client: s.client, Upstream: o.Protocol, Stage: StageRequest, Compatibility: Full}}
	if native {
		a.body = raw
	} else {
		var err error
		a.body, a.mapping, err = ConvertRequestMapped(raw, "responses", o.Protocol)
		if err != nil {
			return nil, newTranslationError(s.client, o.Protocol, StageRequest, ConversionReason(err), err)
		}
		a.diagnostics.Degradations = requestDegradations(raw, o.Protocol)
		if len(a.diagnostics.Degradations) > 0 {
			a.diagnostics.Compatibility = Degraded
		}
	}
	return a, nil
}

// RequestBody returns an owned copy. Neither callers nor retry paths can mutate
// the attempt's baseline. Endpoint is protocol metadata, not a selected URL.
func (a *Attempt) RequestBody() []byte { return append([]byte(nil), a.body...) }
func (a *Attempt) Endpoint() string    { return Endpoint(a.upstream) }
func (a *Attempt) Diagnostics() Diagnostics {
	d := a.diagnostics
	d.Degradations = append([]Degradation(nil), d.Degradations...)
	return d
}
