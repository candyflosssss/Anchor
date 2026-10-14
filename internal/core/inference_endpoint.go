package core

import (
	"bufio"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/candyflosssss/Anchor/internal/protocol"
)

// inferenceResponse tries the configured URL first. Only an HTML page at an
// unversioned base permits one same-origin /v1 attempt. JSON errors, redirects,
// transport failures and event streams never cause a request to be replayed.
// Both attempts share the caller's deadline and concurrency slot.
func (s *Server) inferenceResponse(req *http.Request, base, upstreamProtocol string, evidence map[string]string) (*http.Response, error) {
	evidence["endpoint_resolution"] = "configured"
	evidence["endpoint_requests"] = "1"
	resp, err := s.upstream.Do(req)
	if err != nil {
		return nil, err
	}
	html := inspectInferenceResponse(resp, evidence)
	fallback, ok := inferenceV1URL(base, upstreamProtocol)
	if !html || !ok || req.GetBody == nil || (resp.StatusCode != 200 && resp.StatusCode != 404 && resp.StatusCode != 405) {
		return resp, nil
	}
	body, err := req.GetBody()
	if err != nil {
		return resp, nil
	}
	resp.Body.Close()
	evidence["initial_response_kind"] = "html"
	evidence["initial_http"] = strconv.Itoa(resp.StatusCode)
	evidence["endpoint_resolution"] = "v1_fallback"
	evidence["endpoint_requests"] = "2"
	delete(evidence, "response_kind")
	delete(evidence, "upstream_content_type")
	retry := req.Clone(req.Context())
	retry.URL = fallback
	retry.Body = body
	resp, err = s.upstream.Do(retry)
	if err != nil {
		return nil, err
	}
	inspectInferenceResponse(resp, evidence)
	return resp, nil
}

func inferenceV1URL(base, upstreamProtocol string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || catalogVersionPath.MatchString(strings.TrimRight(u.Path, "/")) {
		return nil, false
	}
	// Append inside an existing deployment prefix, never replace it or change host.
	u.Path = strings.TrimRight(u.Path, "/") + "/v1" + protocol.Endpoint(upstreamProtocol)
	u.RawPath = ""
	return u, true
}

type inferenceBody struct {
	io.Reader
	io.Closer
}

// Keep only fixed structural facts. Never expose the URL, headers or page body.
func inspectInferenceResponse(resp *http.Response, evidence map[string]string) bool {
	media, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	switch media {
	case "text/html", "application/xhtml+xml", "application/json", "text/event-stream", "text/plain", "":
		evidence["upstream_content_type"] = media
	default:
		evidence["upstream_content_type"] = "other"
	}
	html := media == "text/html" || media == "application/xhtml+xml"
	if media != "text/event-stream" && (resp.StatusCode == 200 || resp.StatusCode == 404 || resp.StatusCode == 405) {
		// Confirm a page from its body even when Content-Type says HTML: a
		// mislabelled JSON result must not replay inference. Also recognize SPA
		// fallbacks with missing/wrong types, preserving all inspected bytes.
		reader := bufio.NewReaderSize(resp.Body, 512)
		html = inferenceHTMLPrefix(reader)
		resp.Body = &inferenceBody{Reader: reader, Closer: resp.Body}
	}
	if html {
		evidence["response_kind"] = "html"
	} else {
		evidence["response_kind"] = "api_or_unknown"
	}
	return html
}

// Stop as soon as the prefix cannot be HTML, so a JSON or an unlabelled SSE
// response never waits for the full sniff window to fill.
func inferenceHTMLPrefix(reader *bufio.Reader) bool {
	for n := 1; n <= 512; n++ {
		prefix, err := reader.Peek(n)
		raw := strings.TrimLeft(string(prefix), " \t\r\n")
		// Check partial BOM bytes before Unicode case folding replaces them.
		if err == nil && raw != "" && strings.HasPrefix("\ufeff", raw) {
			continue
		}
		text := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff")))
		if text != "" {
			possible := false
			for _, marker := range []string{"<!doctype html", "<html"} {
				if strings.HasPrefix(text, marker) {
					return true
				}
				possible = possible || strings.HasPrefix(marker, text)
			}
			if !possible {
				return false
			}
		}
		if err != nil {
			return false
		}
	}
	return false
}
