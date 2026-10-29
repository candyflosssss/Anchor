package protocol

import (
	"errors"
)

type Stage string

const (
	StageRequest  Stage = "request"
	StageResponse Stage = "response"
	StageStream   Stage = "stream"
)

type Compatibility string

const (
	Full        Compatibility = "full"
	Degraded    Compatibility = "degraded"
	Unsupported Compatibility = "unsupported"
)

type Degradation string

const (
	SummaryStyle               Degradation = "reasoning_summary_style"
	EncryptedProjection        Degradation = "encrypted_reasoning_projection"
	GrammarAsDescription       Degradation = "custom_grammar_description"
	PromptCacheControl         Degradation = "prompt_cache_control"
	ToolResultErrorText        Degradation = "tool_result_error_text"
	AnthropicThinkingPlaintext Degradation = "anthropic_thinking_plaintext"
)

// Diagnostics contains code-owned facts only. Dispatch and HTTP delivery belong
// to the gateway, which combines these facts with its transport observations.
type Diagnostics struct {
	Client, Upstream string
	Stage            Stage
	Category         string
	Compatibility    Compatibility
	Degradations     []Degradation
}

// TranslationError deliberately never renders the underlying error, which can
// contain arbitrary user fields or upstream text. HTTP adapters retain existing codes.
type TranslationError struct {
	Facts Diagnostics
	cause error
}

func (e *TranslationError) Error() string {
	return "protocol translation: " + string(e.Facts.Stage) + "/" + e.Facts.Category
}
func (e *TranslationError) Unwrap() error { return e.cause }
func newTranslationError(client, upstream string, stage Stage, category string, cause error) *TranslationError {
	if !knownProtocol(client) && client != "" {
		client = "unknown"
	}
	if !knownProtocol(upstream) && upstream != "" {
		upstream = "unknown"
	}
	return &TranslationError{Facts: Diagnostics{Client: client, Upstream: upstream, Stage: stage, Category: category, Compatibility: Unsupported}, cause: cause}
}
func ErrorDiagnostics(err error) Diagnostics {
	var e *TranslationError
	if errors.As(err, &e) {
		return e.Facts
	}
	return Diagnostics{}
}

func requestDegradations(raw []byte, to string) []Degradation {
	if to != "chat-completions" && to != "anthropic" {
		return nil
	}
	m, _ := Decode(raw)
	var d []Degradation
	if to == "chat-completions" && obj(m["reasoning"])["summary"] != nil {
		d = append(d, SummaryStyle)
	}
	if len(arr(m["include"])) > 0 {
		d = append(d, EncryptedProjection)
	}
	var grammar bool
	var visit func([]any)
	visit = func(tools []any) {
		for _, v := range tools {
			t := obj(v)
			if t["type"] == "namespace" {
				visit(arr(t["tools"]))
			}
			if t["type"] == "custom" && obj(t["format"])["type"] == "grammar" {
				grammar = true
			}
		}
	}
	visit(arr(m["tools"]))
	for _, v := range arr(m["input"]) {
		if obj(v)["type"] == "additional_tools" {
			visit(arr(obj(v)["tools"]))
		}
	}
	if grammar {
		d = append(d, GrammarAsDescription)
	}
	return d
}

// addDegradation keeps response-time losses visible together with request facts.
func (d *Diagnostics) addDegradation(label Degradation) {
	if d == nil {
		return
	}
	for _, existing := range d.Degradations {
		if existing == label {
			return
		}
	}
	d.Degradations = append(d.Degradations, label)
	d.Compatibility = Degraded
}
