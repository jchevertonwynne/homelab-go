// Package render executes an html/template into a buffer before writing it.
//
// The response is then either the whole page or a clean 500. Executing
// straight into the ResponseWriter commits a 200 and whatever bytes rendered
// before the failure, which is a truncated page the client has no way to
// recognise as broken.
package render

import (
	"bytes"
	"html/template"
	"net/http"
)

// HTML executes t and writes the result. On failure it responds 500 and
// returns the error, having written nothing partial; the caller logs it,
// because what is safe to put in a log line differs per app.
func HTML(w http.ResponseWriter, t *template.Template, data any) error {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return err
	}
	return write(w, buf.Bytes())
}

// Named is HTML for a specific template in a set, for callers using
// ExecuteTemplate.
func Named(w http.ResponseWriter, t *template.Template, name string, data any) error {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return err
	}
	return write(w, buf.Bytes())
}

func write(w http.ResponseWriter, b []byte) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The error here is a client that went away mid-write. Nothing can be
	// done about it, but returning it lets the caller decide whether that is
	// worth a log line.
	_, err := w.Write(b)
	return err
}
