package protocol

import "errors"

// ValidateProbeResponse checks a minimal inference result without requiring that
// its reasoning items and text metadata can be represented in another protocol.
// A token-limited response may contain reasoning only and still prove execution.
func ValidateProbeResponse(raw []byte, p string) (Usage, error) {
	r, err := ParseProbeResponse(raw, p)
	return r.Usage, err
}

// ParseProbeResponse retains validated output for transient diagnostic display.
func ParseProbeResponse(raw []byte, p string) (Result, error) { return parseResponse(raw, p, true) }

type responseValidationError struct {
	reason string
	err    error
}

func (e *responseValidationError) Error() string { return e.err.Error() }
func (e *responseValidationError) Unwrap() error { return e.err }

func validationFailure(reason string, err error) error {
	return &responseValidationError{reason: reason, err: err}
}

// ResponseValidationReason returns only fixed codes suitable for safe logs.
// The original error can include an arbitrary upstream field name.
func ResponseValidationReason(err error) string {
	if err == nil {
		return ""
	}
	var failure *responseValidationError
	if errors.As(err, &failure) {
		return failure.reason
	}
	return "invalid_structure"
}

func responseText(v any, probe bool) (string, error) {
	if !probe {
		return text(v)
	}
	parts, ok := v.([]any)
	if !ok {
		return text(v)
	}
	plain := make([]any, 0, len(parts))
	for _, part := range parts {
		m := obj(part)
		if err := allowed(m, "type text annotations logprobs"); err != nil {
			return "", err
		}
		// Metadata is irrelevant to this execution check. Validate its container
		// without converting, retaining, or logging annotations or token details.
		for _, key := range []string{"annotations", "logprobs"} {
			if metadata := m[key]; metadata != nil {
				if _, ok := metadata.([]any); !ok {
					return "", ErrUnsupported
				}
			}
		}
		plain = append(plain, Object{"type": m["type"], "text": m["text"]})
	}
	return text(plain)
}
