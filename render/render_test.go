package render

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNamedWritesThePageAndContentType(t *testing.T) {
	tmpl := template.Must(template.New("page").Parse(`<p>{{.}}</p>`))
	rec := httptest.NewRecorder()
	if err := Named(rec, tmpl, "page", "hi"); err != nil {
		t.Fatalf("Named: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Body.String(); got != "<p>hi</p>" {
		t.Errorf("body = %q", got)
	}
}

// The reason this package exists: a template that fails halfway must not
// produce a 200 with a truncated page.
func TestNamedWritesNothingPartialOnFailure(t *testing.T) {
	tmpl := template.Must(template.New("page").Parse(`before{{.Missing}}after`))
	rec := httptest.NewRecorder()

	err := Named(rec, tmpl, "page", struct{}{})
	if err == nil {
		t.Fatal("expected an error from a template that cannot execute")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "before") {
		t.Errorf("partial render reached the client: %q", rec.Body.String())
	}
}

func TestHTMLUsesExecute(t *testing.T) {
	// notes calls Execute rather than ExecuteTemplate, so both forms exist.
	tmpl := template.Must(template.New("whatever").Parse(`<h1>{{.}}</h1>`))
	rec := httptest.NewRecorder()
	if err := HTML(rec, tmpl, "x"); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	if got := rec.Body.String(); got != "<h1>x</h1>" {
		t.Errorf("body = %q", got)
	}
}
