package state

import (
	"github.com/candyflosssss/Anchor/internal/naming"
)

func ValidModelID(id string) bool { return naming.ValidModelID(id) }
func ValidServiceTier(tier string) bool {
	return tier == "" || tier == "priority" || tier == "ultrafast"
}

// SpeedVariantID is an explicit creation convention, never a request-time parser.
func SpeedVariantID(base, speed string) (string, string, error) {
	if !ValidID(base) {
		return "", "", ErrInvalid
	}
	tier := ""
	switch speed {
	case "fast":
		tier = "priority"
	case "ultrafast":
		tier = "ultrafast"
	default:
		return "", "", ErrInvalid
	}
	id := base + "(" + speed + ")"
	if !ValidModelID(id) {
		return "", "", ErrInvalid
	}
	return id, tier, nil
}

func speedModels(m Model, providerID string, speeds []string) ([]Model, error) {
	if len(speeds) > 2 || (len(speeds) > 0 && m.ServiceTier != "") {
		return nil, ErrInvalid
	}
	out := []Model{m}
	seen := map[string]bool{}
	for _, speed := range speeds {
		id, tier, err := SpeedVariantID(m.ID, speed)
		if err != nil || seen[speed] {
			return nil, ErrInvalid
		}
		seen[speed] = true
		clone := m
		clone.ID, clone.ServiceTier, clone.Revision = id, tier, 0
		name := m.DisplayName
		if name == "" {
			name = m.ID
		}
		clone.DisplayName = name + "(" + speed + ")"
		// A provider-scoped action does not claim speed support for other providers.
		if providerID != "" {
			clone.Candidates = nil
			for _, c := range m.Candidates {
				if c.ProviderID == providerID {
					clone.Candidates = append(clone.Candidates, c)
				}
			}
		}
		// Standard reference prices do not establish Fast/Ultrafast prices.
		if m.Settings != nil {
			settings := *m.Settings
			settings.Reference = nil
			clone.Settings = &settings
		}
		if clone.Validate() != nil {
			return nil, ErrInvalid
		}
		out = append(out, clone)
	}
	return out, nil
}
