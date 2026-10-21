package protocol

import (
	"encoding/json"

	"github.com/candyflosssss/Anchor/internal/apierror"
)

// RequestInfo exposes common routing facts without making a protocol-shaped map
// the shared semantic representation. Model access and route selection stay in core.
type RequestInfo struct {
	Model     string
	Stream    bool
	ToolCount int
}
type Capabilities struct{ Streaming, Tools bool }

type ingressError struct {
	*TranslationError
	id apierror.ID
}

func (e *ingressError) Unwrap() error { return e.TranslationError }

func (s *Session) ingressFailure(id apierror.ID, category string) error {
	return &ingressError{TranslationError: newTranslationError(s.client, "", StageRequest, category, ErrUnsupported), id: id}
}
func RequestErrorID(err error) apierror.ID {
	if e, ok := err.(*ingressError); ok {
		return e.id
	}
	return apierror.UnsupportedConversion
}

func (s *Session) RequestInfo() (RequestInfo, error) {
	fields := s.PolicyFields()
	var info RequestInfo
	if json.Unmarshal(fields["model"], &info.Model) != nil {
		return info, s.ingressFailure(apierror.ModelIDRequired, "model")
	}
	if raw, ok := fields["stream"]; ok {
		if json.Unmarshal(raw, &info.Stream) != nil {
			return info, s.ingressFailure(apierror.InvalidStream, "stream")
		}
	}
	for _, item := range []struct {
		field string
		id    apierror.ID
	}{
		{"previous_response_id", apierror.PreviousResponseUnsupported}, {"conversation", apierror.ConversationUnsupported},
	} {
		if raw, ok := fields[item.field]; ok && string(raw) != "null" {
			return info, s.ingressFailure(item.id, "stateful_request")
		}
	}
	for _, item := range []struct {
		field string
		id    apierror.ID
	}{
		{"store", apierror.StoreUnsupported}, {"background", apierror.BackgroundUnsupported},
	} {
		if raw, ok := fields[item.field]; ok {
			var enabled bool
			if json.Unmarshal(raw, &enabled) != nil || enabled {
				return info, s.ingressFailure(item.id, "stateful_request")
			}
		}
	}
	tools, err := s.clientTools()
	if err != nil {
		return info, err
	}
	info.ToolCount = len(tools)
	return info, nil
}

func (s *Session) clientTools() ([]json.RawMessage, error) {
	fields := s.PolicyFields()
	var tools []json.RawMessage
	if raw, ok := fields["tools"]; ok && string(raw) != "null" {
		if json.Unmarshal(raw, &tools) != nil {
			return nil, s.ingressFailure(apierror.InvalidTools, "tools")
		}
	}
	// Inline declarations have exactly the same execution/capability policy as
	// top-level tools; a native pass-through is not an escape from that guard.
	if s.client == "responses" {
		var input []json.RawMessage
		if json.Unmarshal(fields["input"], &input) == nil {
			for _, raw := range input {
				var item struct {
					Type  string          `json:"type"`
					Tools json.RawMessage `json:"tools"`
				}
				if json.Unmarshal(raw, &item) != nil || item.Type != "additional_tools" {
					continue
				}
				var extra []json.RawMessage
				if json.Unmarshal(item.Tools, &extra) != nil {
					return nil, s.ingressFailure(apierror.InvalidTools, "tools")
				}
				tools = append(tools, extra...)
			}
		}
	}
	return tools, nil
}

func (s *Session) CheckCapabilities(c Capabilities) error {
	info, err := s.RequestInfo()
	if err != nil {
		return err
	}
	if info.Stream && !c.Streaming {
		return s.ingressFailure(apierror.StreamingUnsupported, "streaming_disabled")
	}
	tools, err := s.clientTools()
	if err != nil {
		return err
	}
	if len(tools) > 0 && !c.Tools {
		return s.ingressFailure(apierror.ToolsUnsupported, "tools_disabled")
	}
	for _, tool := range tools {
		if !AgentExecutedTool(tool, false) {
			return s.ingressFailure(apierror.UnsupportedTool, "hosted_tool")
		}
	}
	return nil
}
