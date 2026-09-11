package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// The collectors are package-level, so tests share them and must not run in
// parallel. Each test uses a distinct HTTP method so its label sets cannot
// collide with another's.

func scrape(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	b, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read exposition: %v", err)
	}
	return string(b)
}

func TestRouteLabelComesFromTheMuxPattern(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("MKCOL /entries/{id}", func(w http.ResponseWriter, r *http.Request) {})

	h := Instrument(mux)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("MKCOL", "/entries/42", nil))

	got := scrape(t)
	// The pattern, not the raw path: /entries/42 as a label would be one
	// series per row ever created.
	if !strings.Contains(got, `route="MKCOL /entries/{id}"`) {
		t.Errorf("pattern not used as the route label:\n%s", filterHTTP(got))
	}
	if strings.Contains(got, "/entries/42") {
		t.Errorf("raw path leaked into a label:\n%s", filterHTTP(got))
	}
}

func TestUnmatchedRoutesCollapseToOneLabel(t *testing.T) {
	h := Instrument(http.NewServeMux())
	for _, p := range []string{"/.env", "/wp-login.php", "/a/b/c"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("REPORT", p, nil))
	}

	got := scrape(t)
	if !strings.Contains(got, `method="REPORT",route="unmatched"`) {
		t.Errorf(`expected route="unmatched":\n%s`, filterHTTP(got))
	}
	for _, p := range []string{".env", "wp-login", "/a/b/c"} {
		if strings.Contains(got, p) {
			t.Errorf("scan path %q became a label — cardinality is unbounded", p)
		}
	}
}

func TestWithRoutersOverridesTheOuterMux(t *testing.T) {
	inner := http.NewServeMux()
	inner.HandleFunc("PROPFIND /items/{id}", func(w http.ResponseWriter, r *http.Request) {})
	outer := http.NewServeMux()
	outer.Handle("/", inner)

	// The outer mux matches "/" for everything; the inner one knows the real
	// pattern, and must win.
	h := Instrument(outer, WithRouters(inner))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PROPFIND", "/items/7", nil))

	got := scrape(t)
	if !strings.Contains(got, `route="PROPFIND /items/{id}"`) {
		t.Errorf("inner pattern did not win:\n%s", filterHTTP(got))
	}
}

func TestStatusIsCapturedAndDefaultsTo200(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /teapot", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	// Writes without WriteHeader, so net/http sends 200 implicitly.
	mux.HandleFunc("PATCH /implicit", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	h := Instrument(mux)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PATCH", "/teapot", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PATCH", "/implicit", nil))

	got := scrape(t)
	if !strings.Contains(got, `route="PATCH /teapot",status="418"`) {
		t.Errorf("418 not recorded:\n%s", filterHTTP(got))
	}
	if !strings.Contains(got, `route="PATCH /implicit",status="200"`) {
		t.Errorf("implicit 200 not recorded:\n%s", filterHTTP(got))
	}
}

func TestExcludedRoutesTouchNeitherHistogramNorGauge(t *testing.T) {
	mux := http.NewServeMux()
	var inFlightDuringRequest float64
	mux.HandleFunc("SEARCH /live", func(w http.ResponseWriter, r *http.Request) {
		// Read the gauge from inside the handler: after it returns the
		// deferred Dec would hide an increment that did happen.
		inFlightDuringRequest = testutil.ToFloat64(inFlight)
	})

	h := Instrument(mux, WithExcludedRoutes("SEARCH /live"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("SEARCH", "/live", nil))

	if inFlightDuringRequest != 0 {
		t.Errorf("in-flight gauge = %v during an excluded route, want 0 — a stream open for hours would make it lie that long", inFlightDuringRequest)
	}
	// Asserted against the exposition rather than WithLabelValues, which would
	// create the very series the test is claiming does not exist.
	if got := scrape(t); strings.Contains(got, `route="SEARCH /live"`) {
		t.Errorf("excluded route was recorded:\n%s", filterHTTP(got))
	}
}

func TestExcludedRouteStillReachesTheHandler(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("SEARCH /served", func(w http.ResponseWriter, r *http.Request) { called = true })

	Instrument(mux, WithExcludedRoutes("SEARCH /served")).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("SEARCH", "/served", nil))

	if !called {
		t.Error("excluding a route from metrics must not stop it being served")
	}
}

func TestStatusWriterUnwrapsForResponseController(t *testing.T) {
	mux := http.NewServeMux()
	var flushErr error
	mux.HandleFunc("LOCK /stream", func(w http.ResponseWriter, r *http.Request) {
		// Fails with ErrNotSupported, silently, if statusWriter loses Unwrap.
		flushErr = http.NewResponseController(w).Flush()
	})

	Instrument(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("LOCK", "/stream", nil))

	if flushErr != nil {
		t.Errorf("Flush through statusWriter: %v — SSE cannot tolerate this", flushErr)
	}
}

func TestHandlerServesTheDefaultRegistry(t *testing.T) {
	got := scrape(t)
	// A wrong registry would serve a valid but empty page, so check for a
	// collector this package never registers itself.
	if !strings.Contains(got, "go_goroutines") {
		t.Error("go_goroutines absent — Handler is not serving the default registry")
	}
}

// filterHTTP trims a scrape to the http_ series, so a failure message is not
// buried in go_memstats_*.
func filterHTTP(exposition string) string {
	var b strings.Builder
	for _, l := range strings.Split(exposition, "\n") {
		if strings.HasPrefix(l, "http_") {
			b.WriteString(l)
			b.WriteString("\n")
		}
	}
	return b.String()
}
