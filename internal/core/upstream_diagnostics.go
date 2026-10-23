package core

import (
	"bufio"
	"io"
	"net/http"

	"github.com/candyflosssss/Anchor/internal/protocol"
)

// Extract only recognized error categories from a bounded JSON prefix. Restore
// every byte for the existing recovery detector. Remote messages are not logs.
func captureUpstreamError(resp *http.Response, evidence map[string]string) {
	reader := bufio.NewReaderSize(resp.Body, 16<<10)
	prefix, readErr := reader.Peek(16 << 10)
	resp.Body = &inferenceBody{Reader: reader, Closer: resp.Body}
	if readErr != nil && readErr != io.EOF {
		return
	}
	if category := protocol.ClassifyUpstreamError(prefix); category != "" {
		evidence["upstream_error_category"] = category
	}
}
