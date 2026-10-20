package apierror

import "net/http"

// WithLanguage carries request-scoped error language, including before auth.
// Unwrap preserves ResponseController flushing and write deadlines for SSE.
func WithLanguage(w http.ResponseWriter, language Language) http.ResponseWriter {
	if language != Chinese {
		language = English
	}
	return &languageWriter{ResponseWriter: w, language: language}
}

type languageWriter struct {
	http.ResponseWriter
	language Language
}

func (w *languageWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func LanguageOf(w http.ResponseWriter) Language {
	for {
		if localized, ok := w.(*languageWriter); ok {
			return localized.language
		}
		wrapped, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return English
		}
		w = wrapped.Unwrap()
	}
}
