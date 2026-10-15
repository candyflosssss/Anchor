package managed

import "errors"

// Stable categories for safe API diagnostics. Parser/filesystem causes remain
// wrapped locally and must never be copied into public error messages.
var (
	ErrModelUnavailable = errors.New("managed default model is unavailable")
	ErrCatalogInvalid   = errors.New("managed model catalog could not be generated")
)
