package catalog

import (
	"github.com/candyflosssss/Anchor/internal/modelconfig"
	"github.com/candyflosssss/Anchor/internal/modeldev"
	"regexp"
)

var modelSettingsID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

type ModelSettingsQuery struct {
	ModelID string `json:"model_id"`
}
type ModelSettingsState struct {
	ModelID        string                         `json:"model_id"`
	Revision       int64                          `json:"revision"`
	DisplayName    string                         `json:"display_name"`
	CandidateCount int                            `json:"candidate_count"`
	Configuration  modelconfig.ModelConfiguration `json:"configuration"`
	Reference      *modeldev.ModelCatalog         `json:"reference"`
}

// Configuration is replaced explicitly. Reference is independently kept,
// replaced using exact public catalog identity, or cleared.
type SaveModelSettings struct {
	ModelID             string                         `json:"model_id"`
	ExpectedRevision    int64                          `json:"expected_revision"`
	DisplayName         string                         `json:"display_name"`
	Configuration       modelconfig.ModelConfiguration `json:"configuration"`
	ReferenceAction     string                         `json:"reference_action"`
	ReferenceProviderID string                         `json:"reference_provider_id"`
	ReferenceModelID    string                         `json:"reference_model_id"`
}

func (in SaveModelSettings) Validate() error {
	if !modelSettingsID.MatchString(in.ModelID) || in.ExpectedRevision < 1 || in.ExpectedRevision >= modeldev.MaxSafeInteger || len(in.DisplayName) > 256 {
		return modeldev.ErrInvalid
	}
	switch in.ReferenceAction {
	case "keep", "clear":
		if in.ReferenceProviderID != "" || in.ReferenceModelID != "" {
			return modeldev.ErrInvalid
		}
	case "set":
		if in.ReferenceProviderID == "" || in.ReferenceModelID == "" || (modeldev.ModelCatalogQuery{ProviderID: in.ReferenceProviderID, ModelID: in.ReferenceModelID}).Validate() != nil {
			return modeldev.ErrInvalid
		}
	default:
		return modeldev.ErrInvalid
	}
	return in.Configuration.Validate()
}
