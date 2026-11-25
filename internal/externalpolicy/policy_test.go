package externalpolicy

import "testing"

func TestEvaluateMatrix(t *testing.T) {
	internal := []string{"acme.com"}
	allow := []Partner{{Domain: "partner.com", Effect: "allow"}, {Domain: "gmail.com", Effect: "allow"}}
	block := []Partner{{Domain: "evil.com", Effect: "block"}}
	both := append(append([]Partner{}, allow...), block...)
	cases := []struct {
		name     string
		mode     string
		partners []Partner
		email    string
		want     bool
		status   string
	}{
		{"internal always", ModeAllowlist, nil, "bob@acme.com", true, "internal"},
		{"internal case", ModeAllowlist, nil, "Bob@ACME.com", true, "internal"},
		{"allow_all open", ModeAllowAll, nil, "x@random.org", true, "allowed"},
		{"allow_all freemail", ModeAllowAll, nil, "x@gmail.com", true, "allowed"},
		{"allow_all block entry", ModeAllowAll, block, "x@evil.com", false, "blocked"},
		{"allowlist unlisted", ModeAllowlist, allow, "x@random.org", false, "blocked"},
		{"allowlist listed", ModeAllowlist, allow, "x@partner.com", true, "allowed"},
		{"allowlist block wins", ModeAllowlist, both, "x@evil.com", false, "blocked"},
		{"blocklist unlisted", ModeBlocklist, block, "x@random.org", true, "allowed"},
		{"blocklist blocked", ModeBlocklist, block, "x@evil.com", false, "blocked"},
		{"blocklist freemail unlisted", ModeBlocklist, block, "x@yahoo.com", false, "blocked"},
		{"blocklist freemail allow-listed", ModeBlocklist, both, "x@gmail.com", true, "allowed"},
		{"invalid email", ModeAllowAll, nil, "nope", false, "blocked"},
		{"display name form", ModeAllowAll, block, "Eve <eve@evil.com>", false, "blocked"},
	}
	for _, c := range cases {
		p := Default()
		p.DomainMode = c.mode
		v := Evaluate(p, c.partners, internal, c.email)
		if v.Allowed != c.want || v.Status != c.status {
			t.Errorf("%s: got allowed=%v status=%s (%s)", c.name, v.Allowed, v.Status, v.Reason)
		}
	}
	if err := (Policy{GuestInvites: "x"}).Validate(); err == nil {
		t.Error("invalid policy accepted")
	}
	if err := Default().Validate(); err != nil {
		t.Error(err)
	}
}
