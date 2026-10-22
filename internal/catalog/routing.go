package catalog

type SourceRoute struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
	KeyID      string `json:"key_id"`
}

type RouteCandidate struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

// Order changes only the position of existing candidates; it cannot change membership.
type ModelRouting struct {
	SourceOrder      []SourceRoute    `json:"source_order,omitempty"`
	ModelID          string           `json:"model_id"`
	ExpectedRevision int64            `json:"expected_revision"`
	Strategy         string           `json:"strategy"`
	Order            []RouteCandidate `json:"order"`
}
type ModelRoutingState struct {
	SourceOrder []SourceRoute    `json:"source_order,omitempty"`
	ModelID     string           `json:"model_id"`
	Revision    int64            `json:"revision"`
	Strategy    string           `json:"strategy"`
	Order       []RouteCandidate `json:"order"`
}
