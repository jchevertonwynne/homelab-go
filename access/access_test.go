package access

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeEmailCollapsesCaseAndSpace(t *testing.T) {
	for _, in := range []string{"Alice@x.com", " alice@x.com ", "ALICE@X.COM", "\talice@x.com\n"} {
		if got := NormalizeEmail(in); got != "alice@x.com" {
			t.Errorf("NormalizeEmail(%q) = %q", in, got)
		}
	}
}

func TestEmailReadsTheAccessHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(EmailHeader, " Alice@X.com ")
	got, ok := Email(r, "")
	if !ok || got != "alice@x.com" {
		t.Errorf("Email = (%q, %v), want (alice@x.com, true)", got, ok)
	}
}

// devUser must never win over a real Access header: an app behind Access must
// not have its caller overridable by its own flags.
func TestHeaderBeatsDevUser(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(EmailHeader, "real@x.com")
	got, ok := Email(r, "dev@x.com")
	if !ok || got != "real@x.com" {
		t.Errorf("Email = (%q, %v), want the header to win", got, ok)
	}
}

func TestDevUserOnlyFillsInWhenTheHeaderIsAbsent(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	got, ok := Email(r, " Dev@X.com ")
	if !ok || got != "dev@x.com" {
		t.Errorf("Email = (%q, %v), want the normalised devUser", got, ok)
	}
}

// The deny path. ok=false is the only signal; this package does not decide
// what it means.
func TestNoHeaderAndNoDevUserIsNotOK(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if got, ok := Email(r, ""); ok {
		t.Errorf("Email = (%q, true), want ok=false", got)
	}
}

func TestBlankHeaderIsTreatedAsAbsent(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set(EmailHeader, "   ")
	if _, ok := Email(r, ""); ok {
		t.Error("whitespace-only header was accepted as an identity")
	}
}
