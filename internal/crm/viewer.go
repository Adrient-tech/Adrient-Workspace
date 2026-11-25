package crm

import (
	"context"
	"regexp"
)

// Scope is how much of the CRM a user may see.
const (
	ScopeOwn  = "own"
	ScopeTeam = "team"
	ScopeAll  = "all"
)

// Viewer identifies who is asking. Repository methods read it from the context
// and restrict every list/get/write to the records that viewer may see. A
// context without a Viewer (background jobs, tests of the raw store) sees all.
type Viewer struct {
	UserID string
	Scope  string // own | team | all (admins are resolved to all by the caller)
}

type viewerKey struct{}

// WithViewer attaches v to ctx.
func WithViewer(ctx context.Context, v Viewer) context.Context {
	return context.WithValue(ctx, viewerKey{}, v)
}

// ViewerFrom returns the Viewer on ctx (zero value = unrestricted).
func ViewerFrom(ctx context.Context) Viewer {
	v, _ := ctx.Value(viewerKey{}).(Viewer)
	return v
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Unrestricted reports whether the viewer sees every record.
func (v Viewer) Unrestricted() bool {
	return v.UserID == "" || v.Scope == "" || v.Scope == ScopeAll
}

// ownerSQL is the visibility predicate for a table alias that has owner_id and
// created_by columns. The user id is inlined only after it matches a strict
// UUID pattern; anything else yields FALSE (fail closed).
func (v Viewer) ownerSQL(alias string) string {
	if v.Unrestricted() {
		return "TRUE"
	}
	if !uuidRe.MatchString(v.UserID) {
		return "FALSE"
	}
	u := "'" + v.UserID + "'::uuid"
	own := "(" + alias + ".owner_id=" + u + " OR (" + alias + ".owner_id IS NULL AND " + alias + ".created_by=" + u + "))"
	if v.Scope != ScopeTeam {
		return own
	}
	return "(" + own + " OR " + alias + ".owner_id IN (SELECT m2.user_id FROM adrient.crm_team_members m1 " +
		"JOIN adrient.crm_team_members m2 ON m2.team_id=m1.team_id WHERE m1.user_id=" + u + "))"
}

// childSQL is the predicate for rows that hang off deals/people (follow-ups,
// activities): visible when the viewer created them or can see the deal or person.
func (v Viewer) childSQL(alias string) string {
	if v.Unrestricted() {
		return "TRUE"
	}
	if !uuidRe.MatchString(v.UserID) {
		return "FALSE"
	}
	return "(" + alias + ".user_id='" + v.UserID + "'::uuid" +
		" OR EXISTS (SELECT 1 FROM adrient.crm_deals vd WHERE vd.id=" + alias + ".deal_id AND " + v.ownerSQL("vd") + ")" +
		" OR EXISTS (SELECT 1 FROM adrient.crm_people vp WHERE vp.contact_id=" + alias + ".contact_id AND vp.org_id=" + alias + ".org_id AND " + v.ownerSQL("vp") + "))"
}

// CanSee reports whether ownerID/createdBy is visible to v, given the user ids
// that share a team with the viewer. Pure helper used by the unit tests; the
// SQL predicates above implement the same rule.
func (v Viewer) CanSee(ownerID, createdBy string, teammates map[string]bool) bool {
	if v.Unrestricted() {
		return true
	}
	if ownerID == v.UserID || (ownerID == "" && createdBy == v.UserID) {
		return true
	}
	return v.Scope == ScopeTeam && ownerID != "" && teammates[ownerID]
}
