package protocol

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/candyflosssss/Anchor/internal/apierror"
)

func TestConvertedStreamFailuresUseErrorCatalog(t *testing.T) {
	for _, to := range []string{"responses", "anthropic", "chat-completions"} {
		for _, lang := range []apierror.Language{apierror.English, apierror.Chinese} {
			t.Run(to+"/"+string(lang), func(t *testing.T) {
				from := "chat-completions"
				if to == from {
					from = "responses"
				}
				w := httptest.NewRecorder()
				_, err := Stream(apierror.WithLanguage(w, lang), strings.NewReader("data: {}\n\n"), from, to, "fixture")
				body := w.Body.String()
				if err == nil || !strings.Contains(body, "[Anchor] ") || strings.Contains(body, "response.completed") {
					t.Fatalf("missing local failure or emitted success: %v %s", err, body)
				}
				if lang == apierror.Chinese && !strings.Contains(body, "上游流式响应不完整") {
					t.Fatalf("missing Chinese error: %s", body)
				}
				if lang == apierror.English && !strings.Contains(body, "Upstream stream incomplete or unsupported") {
					t.Fatalf("missing English error: %s", body)
				}
				code := "upstream_stream_error"
				if to == "anthropic" {
					code = "api_error"
				}
				if !strings.Contains(body, code) {
					t.Fatalf("protocol error code changed: %s", body)
				}
			})
		}
	}
}
