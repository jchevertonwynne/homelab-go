package access

import (
	"errors"
	"slices"
	"strings"
)

// Allowlist is the set of addresses a hostname's Cloudflare Access policy
// admits, handed to an app at startup.
//
// An app behind Access cannot see that policy. Access refuses everyone else in
// front of it, and the only thing about it that reaches the app is the header
// of whoever got through. So the homelab repo generates the list into a
// ConfigMap and passes it in as a flag.
//
// Checking it is not redundant with Access. Access consults its policy when it
// issues a session and not again afterwards, so taking someone off the policy
// stops their next sign-in while a session they already hold keeps sending
// their header until it expires — up to a month, with the session lengths
// these apps use. An app that checks Admits on every request refuses that
// session the moment its own list no longer names the address, which is when
// the pod restarts onto the new ConfigMap.
//
// Like the rest of this package it decides nothing about a request: it answers
// whether an address is admitted, and the app is left holding the refusal. The
// zero value admits everyone, which is what an unconfigured list means; see
// Check for why production must not run that way.
type Allowlist struct {
	// emails is sorted, deduplicated and normalised. It exists alongside set
	// because an app that offers the list to a person — list's invite
	// dropdown — wants a stable order.
	emails  []string
	set     map[string]bool
	devUser string
}

// NewAllowlist parses the raw comma-separated flag value.
//
// Addresses are normalised exactly as NormalizeEmail normalises the one on a
// request, since the two are compared: without that, a capital letter in a
// ConfigMap would lock someone out. Blank entries are dropped, so a trailing
// comma or a stray newline cannot become an address nobody matches.
//
// devUser is the local-development identity, the same value passed to Email.
// It is admitted whether or not it appears in the list, because it only ever
// stands in for a request that arrived with no header at all: somewhere there
// is no Access, and so no policy to be on. In production it is empty.
func NewAllowlist(raw, devUser string) Allowlist {
	a := Allowlist{set: map[string]bool{}, devUser: NormalizeEmail(devUser)}
	for _, part := range strings.Split(raw, ",") {
		email := NormalizeEmail(part)
		if email == "" || a.set[email] {
			continue
		}
		a.set[email] = true
		a.emails = append(a.emails, email)
	}
	slices.Sort(a.emails)
	return a
}

// Admits reports whether an authenticated address may use the app.
//
// email must already be normalised — it comes from Email, which normalises.
//
// An empty list admits everyone. That is the "not configured" case: a copy of
// deployment configuration that never arrived must not be able to lock out
// everyone the policy does admit, and Check is what keeps production from
// running in that state at all.
func (a Allowlist) Admits(email string) bool {
	if len(a.set) == 0 {
		return true
	}
	if a.devUser != "" && email == a.devUser {
		return true
	}
	return a.set[email]
}

// Emails returns the admitted addresses, sorted, for an app that shows them —
// list offers them as the people who can be invited to a collection. The
// devUser is not among them: it is an identity for a machine with no Access in
// front of it, not a person on the policy.
func (a Allowlist) Emails() []string { return slices.Clone(a.emails) }

// Configured reports whether any address was given. An app that behaves
// differently without a list — falling back to some other set of candidates to
// offer — asks this rather than inferring it from an empty Emails.
func (a Allowlist) Configured() bool { return len(a.set) > 0 }

// DevUser returns the normalised local-development identity, to be passed to
// Email so that both halves agree on who that is.
func (a Allowlist) DevUser() string { return a.devUser }

// Check refuses the one configuration in which an app admits whoever Access
// lets through: no list and no devUser. In production that means the ConfigMap
// never reached the flag, and carrying on would quietly reopen the window
// Admits exists to close. A pod that crashloops says so; one that serves
// everybody does not.
//
// Callers treat this like a database that will not open: log it and exit.
func (a Allowlist) Check() error {
	if !a.Configured() && a.devUser == "" {
		return errors.New("no allowlist and no dev user: refusing to admit everyone Access lets through")
	}
	return nil
}
