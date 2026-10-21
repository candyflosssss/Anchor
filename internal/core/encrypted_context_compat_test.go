package core

import (
	"context"
	"encoding/json"
	"github.com/candyflosssss/Anchor/internal/protocol"
	"io"
)

const encryptedContextPrefixLimit = protocol.RecoveryPrefixLimit

type encryptedContextRecovery struct {
	request map[string]json.RawMessage
	reason  string
	removed int
}
type encryptedContextStreamDetails = protocol.RecoveryStreamDetails

func hasEncryptedReasoning(r map[string]json.RawMessage) bool {
	return protocol.HasEncryptedReasoning(r)
}
func encryptedContextError(b []byte) string { return protocol.EncryptedContextError(b) }
func recoverEncryptedContext(r map[string]json.RawMessage, reason string) *encryptedContextRecovery {
	v := protocol.RecoverEncryptedContext(r, reason)
	if v == nil {
		return nil
	}
	return &encryptedContextRecovery{request: v.Request, reason: v.Reason, removed: v.Removed}
}
func primeEncryptedContextStream(ctx context.Context, r io.Reader, details ...*encryptedContextStreamDetails) (io.Reader, string, string, string) {
	return protocol.PrimeRecoveryStream(ctx, r, details...)
}
