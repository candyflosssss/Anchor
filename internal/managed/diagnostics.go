package managed

import (
	"errors"
	"os"

	"github.com/candyflosssss/Anchor/internal/state"
)

// Diagnostic contains only code-owned labels, never error strings, file paths,
// configuration values or credentials. It is safe to include in system logs.
type Diagnostic struct {
	Stage  string
	Reason string
	Field  string
}

type diagnosticError struct {
	cause  error
	detail Diagnostic
}

func (e *diagnosticError) Error() string { return e.cause.Error() }
func (e *diagnosticError) Unwrap() error { return e.cause }

// Diagnose preserves the most specific annotated cause while keeping unknown
// parser/filesystem errors private. errors.Is continues to classify the cause.
func Diagnose(err error) Diagnostic {
	var annotated *diagnosticError
	if errors.As(err, &annotated) {
		return annotated.detail
	}
	d := Diagnostic{Stage: "operation", Reason: "operation_failed"}
	switch {
	case errors.Is(err, ErrModelUnavailable):
		d.Reason = "model_unavailable"
	case errors.Is(err, ErrCatalogInvalid):
		d.Reason = "catalog_invalid"
	case errors.Is(err, ErrChanged):
		d.Reason = "configuration_changed"
	case errors.Is(err, ErrUnsafe):
		d.Reason = "unsafe_configuration"
	case errors.Is(err, state.ErrConflict):
		d.Reason = "revision_conflict"
	case errors.Is(err, state.ErrNotFound), errors.Is(err, os.ErrNotExist):
		d.Reason = "not_found"
	case errors.Is(err, os.ErrPermission):
		d.Reason = "permission_denied"
	default:
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			d.Reason = "filesystem_error"
		}
	}
	return d
}

func withDiagnostic(err error, stage, reason, field string) error {
	if err == nil {
		return nil
	}
	d := Diagnose(err)
	if stage != "" {
		d.Stage = stage
	}
	if reason != "" {
		d.Reason = reason
	}
	if field != "" {
		d.Field = field
	}
	return &diagnosticError{cause: err, detail: d}
}

// Managed paths can contain user-defined provider IDs. Map them to fixed labels
// rather than serializing a path (or any part of a configuration value).
func diagnosticField(path []string) string {
	if len(path) == 0 {
		return "managed_field"
	}
	switch path[0] {
	case "llm-pi-ai":
		if len(path) == 1 {
			return "dsh.llm_plugin"
		}
		return "dsh.provider"
	case "agent-default-model":
		if len(path) == 1 {
			return "dsh.default_plugin"
		}
		if len(path) == 3 && path[1] == "config" {
			switch path[2] {
			case "model":
				return "dsh.default_model"
			case "provider":
				return "dsh.default_provider"
			case "reasoningEffort":
				return "dsh.default_reasoning"
			}
		}
	case "catalog":
		return "model_catalog_json"
	case "model", "model_provider", "model_providers", "model_catalog_json", "availableModels", "modelPicker", "env", "apiKeyHelper", "inferenceModels", "inferenceProvider", "inferenceGatewayBaseUrl", "inferenceGatewayApiKey", "inferenceCredentialKind":
		return path[0]
	}
	return "managed_field"
}
