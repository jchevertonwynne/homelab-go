package access

import (
	"slices"
	"testing"
)

func TestAllowlistAdmitsOnlyWhatItNames(t *testing.T) {
	a := NewAllowlist("alice@x.com,bob@x.com", "")
	for _, email := range []string{"alice@x.com", "bob@x.com"} {
		if !a.Admits(email) {
			t.Errorf("Admits(%q) = false, want true", email)
		}
	}
	// The case the type exists for: an address the policy no longer names,
	// still presenting a session Access issued before it was removed.
	if a.Admits("removed@x.com") {
		t.Error("Admits(removed@x.com) = true, want false")
	}
}

// A ConfigMap is written by hand and by a generator, so neither case nor
// stray whitespace may decide whether someone can sign in.
func TestAllowlistNormalisesWhatItParses(t *testing.T) {
	a := NewAllowlist("  Alice@X.com , BOB@x.com,,alice@x.com ,\n", "")
	if got := a.Emails(); !slices.Equal(got, []string{"alice@x.com", "bob@x.com"}) {
		t.Fatalf("Emails() = %q, want the two addresses, normalised and deduplicated", got)
	}
	if !a.Admits("alice@x.com") {
		t.Error("an address given in mixed case is not admitted in lower case")
	}
}

// Empty means "not configured", which admits everyone: a copy of config that
// never arrived must not lock out the people the policy does admit. Check is
// what stops production running this way.
func TestEmptyAllowlistAdmitsEveryone(t *testing.T) {
	for _, raw := range []string{"", "  ", ",", " , "} {
		a := NewAllowlist(raw, "")
		if !a.Admits("anyone@x.com") {
			t.Errorf("NewAllowlist(%q).Admits = false, want true", raw)
		}
		if a.Configured() {
			t.Errorf("NewAllowlist(%q).Configured = true", raw)
		}
	}
}

func TestDevUserIsAdmittedWithoutBeingListed(t *testing.T) {
	a := NewAllowlist("alice@x.com", " Dev@Local ")
	if !a.Admits("dev@local") {
		t.Error("the devUser is not admitted")
	}
	if got := a.DevUser(); got != "dev@local" {
		t.Errorf("DevUser() = %q, want the normalised value", got)
	}
	// It is an identity, not a switch that turns the list off.
	if a.Admits("someone@else.com") {
		t.Error("setting a devUser admitted an unlisted address")
	}
	// And it is not a person on the policy, so nothing offers it as one.
	if slices.Contains(a.Emails(), "dev@local") {
		t.Errorf("Emails() = %q, want the devUser left out", a.Emails())
	}
}

func TestEmailsIsACopy(t *testing.T) {
	a := NewAllowlist("alice@x.com,bob@x.com", "")
	a.Emails()[0] = "mallory@x.com"
	if got := a.Emails(); got[0] != "alice@x.com" {
		t.Errorf("Emails() = %q; a caller mutated the list itself", got)
	}
	if a.Admits("mallory@x.com") {
		t.Error("a caller mutating Emails changed who is admitted")
	}
}

func TestCheck(t *testing.T) {
	for _, tc := range []struct {
		name, raw, devUser string
		wantErr            bool
	}{
		{name: "neither", wantErr: true},
		{name: "blank list and blank dev user", raw: " , ", devUser: " ", wantErr: true},
		{name: "list only", raw: "alice@x.com"},
		{name: "dev user only", devUser: "dev@local"},
		{name: "both", raw: "alice@x.com", devUser: "dev@local"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := NewAllowlist(tc.raw, tc.devUser).Check()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Check() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// The zero value is a legitimate state — an app that was handed nothing — and
// must not panic on the nil map.
func TestZeroValueAdmitsEveryone(t *testing.T) {
	var a Allowlist
	if !a.Admits("anyone@x.com") {
		t.Error("the zero value refuses everyone")
	}
	if a.Configured() || a.Emails() != nil || a.Check() == nil {
		t.Error("the zero value does not read as unconfigured")
	}
}
