package protocol

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/candyflosssss/Anchor/internal/apierror"
)

// Legacy HTTP entry points delegate to the same transport-independent converter.
func Stream(w http.ResponseWriter, body io.Reader, from, to, model string) (Usage, error) {
	return StreamObserved(w, body, from, to, model, nil)
}
func StreamObserved(w http.ResponseWriter, body io.Reader, from, to, model string, observation *Observation) (Usage, error) {
	return StreamObservedMapped(w, body, from, to, model, observation, nil)
}
func StreamObservedMapped(w http.ResponseWriter, body io.Reader, from, to, model string, observation *Observation, mapping *ChatMapping) (Usage, error) {
	return StreamObservedOptions(w, body, from, to, model, observation, mapping, StreamOptions{Language: apierror.LanguageOf(w)})
}

// StreamObservedOptions keeps the legacy probe entry on the same semantic observer.
func StreamObservedOptions(w http.ResponseWriter, body io.Reader, from, to, model string, observation *Observation, mapping *ChatMapping, options StreamOptions) (Usage, error) {
	if observation == nil {
		observation = &Observation{}
	}
	observation.onFirstOutput = options.OnFirstOutput
	defer func() { observation.onFirstOutput = nil }()
	return translateStream(HTTPStreamWriter(w), body, from, to, model, observation, mapping, options)
}

// HTTPStreamWriter supplies delivery deadlines and flushing at the HTTP edge.
// Attempt.TranslateStream itself accepts ordinary readers and writers.
func HTTPStreamWriter(w http.ResponseWriter) io.Writer { return &httpStreamWriter{w: w} }

type httpStreamWriter struct{ w http.ResponseWriter }

func (w *httpStreamWriter) Write(p []byte) (int, error) {
	if err := http.NewResponseController(w.w).SetWriteDeadline(time.Now().Add(15 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return 0, err
	}
	return w.w.Write(p)
}
func (w *httpStreamWriter) Flush() error {
	err := http.NewResponseController(w.w).Flush()
	if errors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return err
}
func flushStream(w io.Writer) error {
	if f, ok := w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	if f, ok := w.(interface{ Flush() }); ok {
		f.Flush()
	}
	return nil
}
