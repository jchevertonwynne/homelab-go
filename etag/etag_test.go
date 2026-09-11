package etag

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestOfIsQuotedAnd32HexChars(t *testing.T) {
	got := Of([]byte("hello"))
	// 16 bytes of digest, hex, in quotes. The length is the convention five
	// copies of this used to each decide for themselves.
	if len(got) != 34 {
		t.Errorf("ETag = %q (len %d), want a quoted 32-char hex string", got, len(got))
	}
	if got[0] != '"' || got[len(got)-1] != '"' {
		t.Errorf("ETag = %q, want it quoted — an unquoted value is a weak validator", got)
	}
}

func TestOfIsContentDerived(t *testing.T) {
	if Of([]byte("a")) == Of([]byte("b")) {
		t.Error("different content produced the same ETag")
	}
	// Determinism across calls, which is what makes an unchanged file still
	// answer 304 after a deploy. Via variables, so staticcheck does not read
	// it as comparing an expression with itself.
	first, second := Of([]byte("a")), Of([]byte("a"))
	if first != second {
		t.Errorf("same content produced %q then %q — a deploy would break every cache", first, second)
	}
}

func TestMapKeysByURLPath(t *testing.T) {
	fsys := fstest.MapFS{
		"static/style.css":     {Data: []byte("body{}")},
		"static/sub/icon.svg":  {Data: []byte("<svg/>")},
		"elsewhere/ignore.txt": {Data: []byte("no")},
	}
	m, err := Map(fsys, "static")
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if _, ok := m["/static/style.css"]; !ok {
		t.Errorf("missing /static/style.css; got keys %v", m)
	}
	if _, ok := m["/static/sub/icon.svg"]; !ok {
		t.Errorf("Map did not recurse; got keys %v", m)
	}
	if _, ok := m["/elsewhere/ignore.txt"]; ok {
		t.Error("Map walked outside root")
	}
}

func TestHandlerAnswers304OnIfNoneMatch(t *testing.T) {
	fsys := fstest.MapFS{"static/style.css": {Data: []byte("body{}")}}
	m, err := Map(fsys, "static")
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	h := Handler(fsys, m)

	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/static/style.css", nil))
	tag := first.Header().Get("ETag")
	if tag == "" {
		t.Fatal("no ETag on the first response — FileServerFS cannot supply one for an embed.FS, which is why the map exists")
	}
	if got := first.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}

	// The whole point: a repeat request costs a 304, not the file.
	repeat := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	repeat.Header.Set("If-None-Match", tag)
	second := httptest.NewRecorder()
	h.ServeHTTP(second, repeat)
	if second.Code != http.StatusNotModified {
		t.Errorf("repeat request = %d, want 304", second.Code)
	}
}

func TestServeBytesAnswers304OnIfNoneMatch(t *testing.T) {
	content := []byte("<svg/>")
	tag := Of(content)

	repeat := httptest.NewRequest(http.MethodGet, "/icon.svg", nil)
	repeat.Header.Set("If-None-Match", tag)
	rec := httptest.NewRecorder()
	ServeBytes(rec, repeat, "icon.svg", content, tag)
	if rec.Code != http.StatusNotModified {
		t.Errorf("got %d, want 304", rec.Code)
	}
	// No Last-Modified: the bytes are compiled in and have no modtime, which
	// is the reason the ETag carries the whole job.
	if got := rec.Header().Get("Last-Modified"); got != "" {
		t.Errorf("Last-Modified = %q, want none", got)
	}
}
