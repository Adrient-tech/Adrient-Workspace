package docs

import "testing"

func TestLinkAllowedDocs(t *testing.T) {
	g := func(vis, aud string) ShareGrant {
		return ShareGrant{Share: Share{Visibility: vis, Audience: aud}, DocOrgID: "o1", OwnerID: "own"}
	}
	anon := LinkViewer{}
	mem := LinkViewer{SignedIn: true, UserID: "u", Email: "a@x.test", OrgID: "o1"}
	out := LinkViewer{SignedIn: true, UserID: "v", Email: "b@y.test", OrgID: "o2"}
	if !LinkAllowed(g(VisAnyone, ""), anon) {
		t.Error("anyone")
	}
	if LinkAllowed(g(VisOrgOnly, ""), anon) || LinkAllowed(g(VisOrgOnly, ""), out) || !LinkAllowed(g(VisOrgOnly, ""), mem) {
		t.Error("org_only")
	}
	if LinkAllowed(g(VisNamed, ""), mem) {
		t.Error("named without audience must refuse a plain member")
	}
	if !LinkAllowed(g(VisNamed, "B@Y.test"), out) || LinkAllowed(g(VisNamed, "B@Y.test"), mem) {
		t.Error("named audience match")
	}
	if !LinkAllowed(g(VisNamed, ""), LinkViewer{SignedIn: true, UserID: "own", OrgID: "o1"}) {
		t.Error("owner")
	}
	if LinkAllowed(g("bogus", ""), mem) {
		t.Error("unknown visibility must fail closed")
	}
}
