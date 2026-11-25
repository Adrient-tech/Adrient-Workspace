package drive

import "testing"

func TestLinkAllowed(t *testing.T) {
	anon := LinkViewer{}
	member := LinkViewer{SignedIn: true, UserID: "u1", OrgID: "o1"}
	other := LinkViewer{SignedIn: true, UserID: "u2", OrgID: "o2"}
	grantee := LinkViewer{SignedIn: true, UserID: "u3", OrgID: "o2", IsGrantee: true}
	owner := LinkViewer{SignedIn: true, UserID: "own", OrgID: "o1"}
	cases := []struct {
		vis  string
		v    LinkViewer
		want bool
	}{
		{VisAnyone, anon, true},
		{VisOrgOnly, anon, false}, {VisOrgOnly, member, true}, {VisOrgOnly, other, false},
		{VisNamed, anon, false}, {VisNamed, member, false}, {VisNamed, grantee, true}, {VisNamed, owner, true},
		{"", anon, false}, {"weird", member, false},
	}
	for _, c := range cases {
		err := LinkAllowed(c.vis, "o1", "own", c.v)
		if (err == nil) != c.want {
			t.Errorf("vis=%q viewer=%+v: err=%v want allowed=%v", c.vis, c.v, err, c.want)
		}
	}
	if err := LinkAllowed(VisOrgOnly, "o1", "own", anon); err != errLinkSignIn {
		t.Errorf("anonymous must be asked to sign in, got %v", err)
	}
}
