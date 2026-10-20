package apierror

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestCatalogTranslationsAndFormatParameters(t *testing.T) {
	formats := regexp.MustCompile(`%[ds]`)
	for _, lang := range []Language{English, Chinese} {
		if len(messages[lang]) != len(definitions) {
			t.Fatalf("%s has %d translations for %d errors", lang, len(messages[lang]), len(definitions))
		}
		for id, d := range definitions {
			message := messages[lang][id]
			if message == "" || strings.Contains(message, "[Anchor]") {
				t.Errorf("%s/%s: empty or pre-prefixed message", lang, id)
			}
			if d.status < 400 || d.status > 599 || d.code == "" {
				t.Errorf("%s: invalid definition %+v", id, d)
			}
			got, want := formats.FindAllString(message, -1), formats.FindAllString(messages[English][id], -1)
			if len(got) != len(d.arguments) || strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("%s/%s: incompatible format parameters %v, want %v", lang, id, got, want)
			}
		}
	}
}

func TestLocalizedDetailsKeepWireIdentity(t *testing.T) {
	for _, lang := range []Language{English, Chinese} {
		status, detail := Render(BindingModelDenied, lang)
		if status != http.StatusForbidden || detail.Code != "binding_model_denied" || !strings.HasPrefix(detail.Message, "[Anchor] ") {
			t.Fatalf("%s: %d %+v", lang, status, detail)
		}
		if lang == Chinese && detail.Message != "[Anchor] 请求的模型不在当前 Agent 绑定的允许范围内。" {
			t.Fatalf("Chinese message is not translated: %s", detail.Message)
		}
		_, limit := Render(RequestTooLarge, lang, 32)
		if !strings.Contains(limit.Message, "32 MiB") || strings.Contains(limit.Message, "%!") {
			t.Fatalf("invalid limit formatting: %s", limit.Message)
		}
		_, conversion := Render(CandidateConversionUnsupported, lang, "anthropic", "unsupported_field")
		if !strings.Contains(conversion.Message, "anthropic") || !strings.Contains(conversion.Message, "unsupported_field") {
			t.Fatalf("missing conversion facts: %s", conversion.Message)
		}
	}
	_, english := Render(BindingModelDenied, English)
	_, unknownLanguage := Render(BindingModelDenied, Language("fr"))
	if unknownLanguage != english {
		t.Fatal("unsupported language did not fall back to English")
	}
}

func TestMissingTranslationAndInvalidIdentityFallback(t *testing.T) {
	chinese := messages[Chinese][BindingModelDenied]
	delete(messages[Chinese], BindingModelDenied)
	defer func() { messages[Chinese][BindingModelDenied] = chinese }()
	_, fallback := Render(BindingModelDenied, Chinese)
	_, english := Render(BindingModelDenied, English)
	if fallback != english {
		t.Fatal("missing translation did not fall back to English")
	}
	if fallback.Language != English {
		t.Fatal("fallback reported the wrong message language")
	}
	for _, id := range []ID{ID("private-unregistered-message"), RequestTooLarge} {
		status, detail := Render(id, Chinese)
		if status != 500 || detail.Code != "internal_error" || strings.Contains(detail.Message, "private") {
			t.Fatalf("invalid catalog use leaked details: %d %+v", status, detail)
		}
	}
	for _, args := range [][]any{{"private-error"}, {-1}, {32, "private-error"}} {
		status, detail := Render(RequestTooLarge, Chinese, args...)
		if status != 500 || detail.Code != "internal_error" || strings.Contains(detail.Message, "private-error") || strings.Contains(detail.Message, "%!") {
			t.Fatalf("invalid parameters leaked into a message: %d %+v", status, detail)
		}
	}
}

func TestLanguageNegotiation(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   Language
	}{
		{"", English},
		{"en-US", English},
		{"zh-CN", Chinese},
		{"ZH-hans", Chinese},
		{"zh-TW", Chinese},
		{"fr, zh;q=0.8, en;q=0.5", Chinese},
		{"zh;q=0.5, en-GB;q=0.9", English},
		{"zh;q=0.8, en;q=0.8", Chinese},
		{"zh;q=0, en;q=0.5", English},
		{"zh;q=oops, en", English},
		{"zh;q=1.1, en", English},
		{"zh;q=NaN, en", English},
		{"zh;q=0.1234, en", English},
		{"zh;foo=bar, en", English},
		{"fr-FR, de", English},
		{"*", English},
		{strings.Repeat("zh,", 400), English},
		{strings.Repeat("fr,", 32) + "zh", English},
	} {
		t.Run(tc.header, func(t *testing.T) {
			if got := Negotiate(tc.header); got != tc.want {
				t.Fatalf("language = %s, want %s", got, tc.want)
			}
		})
	}
}
