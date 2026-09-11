// Package access reads the identity Cloudflare Access puts on a request.
//
// It deliberately does not decide anything. There is no middleware here and
// nothing returns a 403, because the interesting half of an authentication
// check is what happens when it fails, and that belongs in the app where a
// reader can see it next to the route it protects. admin refuses a caller who
// is not its owner; list upserts a user row and carries on. Hiding either
// behind a shared Middleware would move a deny decision out of the app that
// makes it.
//
// What is shared is the part that must not drift: the header name, what counts
// as the same address, and — as Allowlist — the set of addresses a hostname's
// Access policy admits, which every app behind Access has to be told and none
// of them can see for itself. Allowlist answers whether an address is on it.
// Refusing the ones that are not is still the app's own line of code.
package access

import (
	"net/http"
	"strings"
)

// EmailHeader is set by Cloudflare Access once it has authenticated the
// caller. Its presence means Access let the request through; its absence in
// production means Access itself stopped working, which is why an app should
// treat that as an error rather than an anonymous visitor.
const EmailHeader = "Cf-Access-Authenticated-User-Email"

// NormalizeEmail collapses case and surrounding whitespace, so "Alice@x.com"
// and " alice@x.com " are one identity.
//
// Access sends the address however the identity provider spelled it, so
// comparing it byte-for-byte to a configured address turns a capital letter
// into a lockout. In the other direction, an address typed by hand that
// disagrees on case would key a second, unreachable account.
func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Email returns the normalised authenticated address.
//
// ok is false only when the header is absent and devUser is empty. The caller
// decides what that means — it is the deny path, and it is not this package's
// to make.
//
// devUser is a local-development fallback and is consulted only when the
// header is missing, never in preference to it: an app running behind Access
// must not be able to have its caller overridden by its own flags.
func Email(r *http.Request, devUser string) (string, bool) {
	if email := NormalizeEmail(r.Header.Get(EmailHeader)); email != "" {
		return email, true
	}
	if devUser := NormalizeEmail(devUser); devUser != "" {
		return devUser, true
	}
	return "", false
}
