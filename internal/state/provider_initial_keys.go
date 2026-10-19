package state

import "strings"

// InitialProviderKey is write-only input for atomic provider creation. Existing
// Keys are edited through ProviderAction, never through a provider replacement.
type InitialProviderKey struct {
	ID       string `json:"id"`
	Protocol string `json:"protocol"`
	Secret   string `json:"secret"`
}

func ValidateInitialProviderKeys(keys []InitialProviderKey, expected int64, secret *string) error {
	if len(keys) == 0 {
		return nil
	}
	if expected != 0 {
		return ErrInvalid
	}
	count := len(keys)
	if secret != nil && *secret != "" {
		count++
	}
	if count > 64 {
		return ErrInvalid
	}
	seen := map[string]bool{"default": true} // The default Key uses the existing secret field.
	for _, key := range keys {
		if !ValidID(key.ID) || seen[key.ID] || !ValidProtocol(key.Protocol) || strings.TrimSpace(key.Secret) == "" || strings.ContainsAny(key.Secret, "\r\n") || len(key.Secret) > 8192 {
			return ErrInvalid
		}
		seen[key.ID] = true
	}
	return nil
}
