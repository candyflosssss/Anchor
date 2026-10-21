package core

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/candyflosssss/Anchor/internal/protocol"
)

// Translation facts contain fixed categories only; transport delivery is recorded
// here, without giving the translator access to request logs or the database.
func recordTranslationDiagnostics(r *http.Request, d protocol.Diagnostics, dispatched, delivered bool) {
	if t := trace(r); t != nil {
		e := t.row.Evidence
		e["translation_client"] = d.Client
		e["conversion_target"] = d.Upstream
		e["translation_stage"] = string(d.Stage)
		e["translation_category"] = d.Category
		e["translation_compatibility"] = string(d.Compatibility)
		e["translation_dispatched"] = strconv.FormatBool(dispatched)
		e["translation_delivered"] = strconv.FormatBool(delivered)
		labels := make([]string, len(d.Degradations))
		for i, label := range d.Degradations {
			labels[i] = string(label)
		}
		e["translation_degradations"] = strings.Join(labels, ",")
	}
}

// Count actual body delivery, including partial writes; headers alone are not content.
type translationDeliveryWriter struct {
	io.Writer
	delivered *bool
}

func (w *translationDeliveryWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	if n > 0 {
		*w.delivered = true
	}
	return n, err
}
func (w *translationDeliveryWriter) Flush() error {
	if f, ok := w.Writer.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}
