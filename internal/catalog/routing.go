package catalog

type RouteCandidate struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

// Order changes only the position of existing candidates; it cannot change membership.
type ModelRouting struct {
	ModelID          string           `json:"model_id"`
	ExpectedRevision int64            `json:"expected_revision"`
	Strategy         string           `json:"strategy"`
	Order            []RouteCandidate `json:"order"`
}
type ModelRoutingState struct {
	ModelID  string           `json:"model_id"`
	Revision int64            `json:"revision"`
	Strategy string           `json:"strategy"`
	Order    []RouteCandidate `json:"order"`
}
