package catalog

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/candyflosssss/Anchor/internal/naming"
)

// Chat is text-only and transient; every turn supplies the complete conversation.
const ChatMaxBytes = 64 << 10
const ChatMaxOutputBytes = 64 << 10
const ChatMaxMessages = 80

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	ProviderID       string        `json:"provider_id"`
	ExpectedRevision int64         `json:"expected_revision"`
	Action           string        `json:"action"`
	KeyID            string        `json:"key_id"`
	Model            string        `json:"model"`
	ReasoningEffort  string        `json:"reasoning_effort,omitempty"`
	RunID            string        `json:"run_id"`
	Approved         bool          `json:"approved"`
	Messages         []ChatMessage `json:"messages"`
}

// Validate does not require the provider to still exist for read/cancel: those
// operations must remain possible after an edit/deletion while a run is active.
func (in ChatRequest) Validate() error {
	if !naming.ValidID(in.ProviderID) || in.ExpectedRevision < 1 {
		return errors.New("invalid chat request")
	}
	switch in.Action {
	case "read", "cancel":
		if in.RunID == "" || len(in.RunID) > 128 {
			return errors.New("invalid chat request")
		}
	case "start":
		if !ValidChatReasoningEffort(in.ReasoningEffort) {
			return errors.New("invalid chat reasoning effort")
		}
		if !in.Approved || in.RunID != "" || in.KeyID == "" || len(in.KeyID) > 512 || in.Model == "" || len(in.Model) > 256 || strings.TrimSpace(in.Model) != in.Model || strings.ContainsAny(in.Model, "\r\n\x00") || !utf8.ValidString(in.Model) {
			return errors.New("invalid chat request")
		}
		if len(in.Messages) == 0 || len(in.Messages) > ChatMaxMessages || in.Messages[len(in.Messages)-1].Role != "user" {
			return errors.New("invalid chat request")
		}
		size := 0
		for i, m := range in.Messages {
			role := "user"
			if i%2 == 1 {
				role = "assistant"
			}
			if m.Role != role || strings.TrimSpace(m.Content) == "" || !utf8.ValidString(m.Content) {
				return errors.New("invalid chat request")
			}
			size += len(m.Content)
		}
		if size > ChatMaxBytes {
			return errors.New("invalid chat request")
		}
	default:
		return errors.New("invalid chat request")
	}
	return nil
}

// Empty preserves the upstream default; supported levels still depend on the model.
func ValidChatReasoningEffort(effort string) bool {
	switch effort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}
