package externalpolicy

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// Gate combines the per-app controls (authz) with the external collaboration
// policy (this package). Drive, Chat, Meet and invitations call the Gate; it is
// the only place the combined rule lives.
type Gate struct {
	Store *Store
	Pool  *pgxpool.Pool
}

type person struct {
	ID, Email, Type, OrgID string
	Found                  bool
}

func (g *Gate) person(ctx context.Context, userID string) person {
	p := person{ID: userID}
	if g.Pool == nil {
		return p
	}
	err := g.Pool.QueryRow(ctx, `SELECT email, user_type, org_id::text FROM adrient.users WHERE id::text=$1`, userID).Scan(&p.Email, &p.Type, &p.OrgID)
	p.Found = err == nil
	return p
}

// isExternalTo reports whether p counts as outside the actor's organization.
func isExternalTo(p person, orgID string) bool {
	return p.Type == authz.TypeGuest || p.Type == authz.TypeExternal || (p.OrgID != "" && p.OrgID != orgID)
}

// verdictFor applies the mode implied by a control to email.
func (g *Gate) verdictFor(ctx context.Context, orgID, email string, open bool, action string) Verdict {
	c, err := g.Store.load(ctx, orgID)
	if err != nil {
		return Verdict{Status: "blocked", Domain: EmailDomain(email), Reason: "Policy could not be read; try again."}
	}
	pol := c.p
	if open {
		pol.DomainMode = ModeAllowAll
	}
	return g.Store.withTrust(ctx, c, orgID, email, action, Evaluate(pol, c.partners, c.internal, email))
}

func actor(ctx context.Context) (orgID string, u authzUser, ok bool) {
	us, ok1 := auth.UserFromContext(ctx)
	o, ok2 := auth.OrgFromContext(ctx)
	if !ok1 || !ok2 {
		return "", authzUser{}, false
	}
	return o.ID, authzUser{ID: us.ID, Type: us.EffectiveType(), Email: us.Email}, true
}

type authzUser struct{ ID, Type, Email string }

// scope maps an enum control value to (deny, open). domains/allowlist = policy
// applies; anyone/on = any partner not explicitly blocked.
func scope(v string) (deny, open bool) {
	switch v {
	case "off":
		return true, false
	case "anyone", "on":
		return false, true
	}
	return false, false
}

// CheckShareToUser decides whether the caller may give userID access to a Drive
// item. Guests need drive.guest_reshare; an external target needs
// drive.external_sharing and an allowing partner policy.
func (g *Gate) CheckShareToUser(ctx context.Context, granteeUserID string) error {
	orgID, me, ok := actor(ctx)
	if !ok {
		return nil
	}
	if me.Type == authz.TypeGuest && !authz.ControlBool(ctx, "drive.guest_reshare") {
		return authz.Denied("drive.guest_reshare", "Guests cannot share items. Ask the person who invited you.")
	}
	t := g.person(ctx, granteeUserID)
	if !t.Found || !isExternalTo(t, orgID) {
		return nil
	}
	return g.externalDecision(ctx, orgID, "drive.external_sharing", t.Email, "share")
}

// CheckLink decides whether the caller may create a share link.
func (g *Gate) CheckLink(ctx context.Context) error {
	_, me, ok := actor(ctx)
	if !ok {
		return nil
	}
	if me.Type == authz.TypeGuest && !authz.ControlBool(ctx, "drive.guest_reshare") {
		return authz.Denied("drive.guest_reshare", "Guests cannot share items. Ask the person who invited you.")
	}
	if authz.ControlEnum(ctx, "drive.external_sharing") == "off" {
		return authz.Denied("drive.external_sharing", "External sharing is turned off for your account, so share links cannot be created.")
	}
	return nil
}

// LinkVisibility returns the visibility a new share link gets, from the
// caller's drive.link_sharing_default control. An unknown value fails closed.
func (g *Gate) LinkVisibility(ctx context.Context) string {
	switch v := authz.ControlEnum(ctx, "drive.link_sharing_default"); v {
	case "anyone", "named", "org_only":
		return v
	}
	return "org_only"
}

func (g *Gate) externalDecision(ctx context.Context, orgID, control, email, action string) error {
	mode := authz.ControlEnum(ctx, control)
	deny, open := scope(mode)
	if deny {
		return authz.Denied(control, "External sharing is turned off for your account.")
	}
	v := g.verdictFor(ctx, orgID, email, open, action)
	if !v.Allowed {
		return authz.Denied(control, v.Reason)
	}
	return nil
}

// CheckChatMembers decides whether the caller may start or extend a chat with
// memberIDs. A conversation is external when the caller or any member is.
func (g *Gate) CheckChatMembers(ctx context.Context, memberIDs []string) error {
	orgID, me, ok := actor(ctx)
	if !ok {
		return nil
	}
	external := me.Type == authz.TypeGuest || me.Type == authz.TypeExternal
	var emails []string
	for _, id := range memberIDs {
		if id == me.ID {
			continue
		}
		t := g.person(ctx, id)
		if t.Found && isExternalTo(t, orgID) {
			external = true
			emails = append(emails, t.Email)
		}
	}
	if !external {
		return nil
	}
	mode := authz.ControlEnum(ctx, "chat.external_chats")
	if mode == "off" {
		return authz.Denied("chat.external_chats", "Chats with external people are turned off for your account.")
	}
	_, open := scope(mode)
	for _, e := range emails {
		if v := g.verdictFor(ctx, orgID, e, open, ActionChat); !v.Allowed {
			return authz.Denied("chat.external_chats", v.Reason)
		}
	}
	return nil
}

// externalConversation reports whether the caller or any member is external,
// and returns the external members' addresses.
func (g *Gate) externalConversation(ctx context.Context, memberIDs []string) (bool, []string) {
	orgID, me, ok := actor(ctx)
	if !ok {
		return false, nil
	}
	external := me.Type == authz.TypeGuest || me.Type == authz.TypeExternal
	var emails []string
	for _, id := range memberIDs {
		if id == me.ID {
			continue
		}
		t := g.person(ctx, id)
		if t.Found && isExternalTo(t, orgID) {
			external = true
			emails = append(emails, t.Email)
		}
	}
	return external, emails
}

// CheckChatUpload enforces chat.file_upload_external: posting attachments to a
// conversation that includes external people needs the control to be on.
func (g *Gate) CheckChatUpload(ctx context.Context, memberIDs []string) error {
	if ext, _ := g.externalConversation(ctx, memberIDs); !ext {
		return nil
	}
	if !authz.ControlBool(ctx, "chat.file_upload_external") {
		return authz.Denied("chat.file_upload_external", "File uploads are turned off in chats that include external people.")
	}
	return nil
}

// CheckEmail checks an e-mail address for invitation create/accept.
func (g *Gate) CheckEmail(ctx context.Context, orgID, email string) error {
	v := g.Store.Check(ctx, orgID, email, ActionInvite)
	if !v.Allowed {
		return authz.Denied("external.partners", v.Reason)
	}
	return nil
}

// CheckMeetJoin decides whether the caller may join a meeting at all: guests
// need meet.guests_allowed and their own domain must not be blocked.
func (g *Gate) CheckMeetJoin(ctx context.Context) error {
	orgID, me, ok := actor(ctx)
	if !ok || (me.Type != authz.TypeGuest && me.Type != authz.TypeExternal) {
		return nil
	}
	if me.Type == authz.TypeGuest && !authz.ControlBool(ctx, "meet.guests_allowed") {
		return authz.Denied("meet.guests_allowed", "Guests are not allowed to join meetings in this organization.")
	}
	if me.Email != "" {
		if v := g.verdictFor(ctx, orgID, me.Email, true, ActionMeet); v.Status == "blocked" {
			return authz.Denied("external.partners", v.Reason)
		}
	}
	return nil
}

// AnonymousMeetAllowed reports whether orgID lets people without an account
// join meetings through a link (control meet.allow_anonymous, resolved for an
// unknown outside visitor: rules for everyone and for the guest user type).
// It fails closed.
func (g *Gate) AnonymousMeetAllowed(ctx context.Context, orgID string) bool {
	c := authz.DefaultControls()
	if c == nil {
		return false
	}
	r, err := c.ResolveFor(ctx, orgID, authz.Subject{UserType: authz.TypeGuest}, "meet.allow_anonymous")
	return err == nil && r.Bool()
}

// Preview answers GET /api/v1/shares/preview: the verdict the share dialog shows.
func (g *Gate) Preview(ctx context.Context, email string) map[string]any {
	orgID, me, ok := actor(ctx)
	if !ok {
		return map[string]any{"verdict": "blocked", "reason": "Not signed in."}
	}
	v := g.Store.Check(ctx, orgID, email, ActionShare)
	out := map[string]any{"domain": v.Domain, "partner": v.Partner, "external": v.External}
	switch {
	case me.Type == authz.TypeGuest && !authz.ControlBool(ctx, "drive.guest_reshare"):
		out["verdict"], out["reason"] = "blocked", "Guests cannot share items."
	case v.Status == "internal":
		out["verdict"], out["reason"] = "allowed", v.Reason
	default:
		mode := authz.ControlEnum(ctx, "drive.external_sharing")
		deny, open := scope(mode)
		if deny {
			out["verdict"], out["reason"] = "blocked", "External sharing is turned off for your account."
		} else if vv := g.verdictFor(ctx, orgID, email, open, ActionShare); !vv.Allowed {
			out["verdict"], out["reason"] = "blocked", vv.Reason
		} else {
			out["verdict"], out["reason"] = "needs_invite", "This person is outside your organization and will be invited as a guest."
		}
	}
	return out
}

// internalDomain reports whether addr belongs to one of the org's verified domains.
func (g *Gate) internalDomain(ctx context.Context, orgID, addr string) bool {
	c, err := g.Store.load(ctx, orgID)
	if err != nil {
		return false
	}
	d := EmailDomain(addr)
	for _, x := range c.internal {
		if strings.EqualFold(x, d) {
			return true
		}
	}
	return false
}

// CheckSend enforces mail.send_external and mail.attachment_max_mb.
func (g *Gate) CheckSend(ctx context.Context, recipients []string, attachmentBytes int64) error {
	orgID, _, ok := actor(ctx)
	if !ok {
		return nil
	}
	if limit := authz.ControlNumber(ctx, "mail.attachment_max_mb"); limit > 0 && float64(attachmentBytes) > limit*1024*1024 {
		return authz.Denied("mail.attachment_max_mb", "Attachments are limited to "+trimFloat(limit)+" MB for your account.")
	}
	if authz.ControlBool(ctx, "mail.send_external") {
		return nil
	}
	for _, a := range recipients {
		if strings.TrimSpace(a) != "" && !g.internalDomain(ctx, orgID, a) {
			return authz.Denied("mail.send_external", "Sending mail outside your organization is turned off for your account.")
		}
	}
	return nil
}

// CheckForward enforces mail.external_forwarding on a forwarding target.
func (g *Gate) CheckForward(ctx context.Context, addr string) error {
	orgID, _, ok := actor(ctx)
	if !ok || authz.ControlBool(ctx, "mail.external_forwarding") {
		return nil
	}
	if !g.internalDomain(ctx, orgID, addr) {
		return authz.Denied("mail.external_forwarding", "Forwarding mail outside your organization is turned off for your account.")
	}
	return nil
}

func trimFloat(f float64) string {
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
	return s
}

// CheckInvite applies the policy to an invitation (userType guest/external/member)
// or an acceptance (userType ""). Members and acceptances are refused only for an
// explicitly blocked domain; guest/external invitations also follow the domain
// mode and the guest-invite switch.
func (g *Gate) CheckInvite(ctx context.Context, orgID, email, userType string) error {
	c, err := g.Store.load(ctx, orgID)
	if err != nil {
		return authz.Denied("external.partners", "Policy could not be read; try again.")
	}
	if userType == "guest" || userType == "external" {
		if !g.InviterAllowed(ctx, c.p.GuestInvites) {
			return authz.Denied("external.policy", inviterReason(c.p.GuestInvites))
		}
	}
	if userType == "guest" || userType == "external" {
		if v := g.Store.withTrust(ctx, c, orgID, email, ActionInvite, Evaluate(c.p, c.partners, c.internal, email)); !v.Allowed {
			return authz.Denied("external.partners", v.Reason)
		}
		return nil
	}
	pol := c.p
	pol.DomainMode = ModeAllowAll
	if v := Evaluate(pol, c.partners, c.internal, email); !v.Allowed {
		return authz.Denied("external.partners", v.Reason)
	}
	return nil
}

func inviterReason(level string) string {
	switch level {
	case "none":
		return "Inviting guests and external users is turned off by your external collaboration policy."
	case "admins":
		return "Only admins can invite guests and external users."
	case "admins_and_sponsors":
		return "Only admins and guest sponsors can invite guests and external users."
	}
	return "You are not allowed to invite guests and external users."
}

// InviterAllowed reports whether the caller may invite guests or external users
// under the policy level (external_policy.guest_invites):
//
//	none                 nobody
//	admins               organization admins
//	admins_and_sponsors  admins, people who already sponsor someone, department heads
//	members              admins and internal members
//	everyone             admins, internal members and external members (never guests)
func (g *Gate) InviterAllowed(ctx context.Context, level string) bool {
	_, me, ok := actor(ctx)
	if !ok {
		return false
	}
	return g.inviterAllowedFor(ctx, level, me.ID, me.Type, authz.IsAdmin(ctx))
}

func (g *Gate) inviterAllowedFor(ctx context.Context, level, userID, userType string, admin bool) bool {
	if level == "none" || userType == authz.TypeGuest || userType == authz.TypeService {
		return false
	}
	if admin {
		return true
	}
	me := authzUser{ID: userID, Type: userType}
	switch level {
	case "everyone":
		return true
	case "members":
		return me.Type == authz.TypeMember || me.Type == ""
	case "admins_and_sponsors":
		if g.Pool == nil {
			return false
		}
		var n int
		_ = g.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM adrient.users WHERE sponsor_id::text=$1)
+ (SELECT count(*) FROM adrient.departments WHERE head_user_id::text=$1)`, me.ID).Scan(&n)
		return n > 0
	}
	return false
}

// ExplainOrgPolicy lists the organization-wide settings that decide what userID
// can do with outside people, each with the answer for that person. It feeds the
// "effective access" explainer in the admin and uses the same functions as
// enforcement.
func (g *Gate) ExplainOrgPolicy(ctx context.Context, orgID, userID, userType string, admin bool) []map[string]any {
	p := g.Store.Policy(ctx, orgID)
	out := []map[string]any{}
	add := func(key, label, value, effect, enforcedAt string) {
		out = append(out, map[string]any{"key": key, "label": label, "value": value, "effect": effect, "enforced_at": enforcedAt})
	}
	can := g.inviterAllowedFor(ctx, p.GuestInvites, userID, userType, admin)
	eff := "cannot invite guests or external users"
	if can {
		eff = "can invite guests and external users"
	}
	add("external.guest_invites", "Who can invite guests", p.GuestInvites, eff, "Invitation create (Admin > Users, invitations API)")
	add("external.domain_mode", "Partner domains", p.DomainMode, "decides which outside domains this person can share with, chat with, meet and invite", "Drive and Docs sharing, Chat members, Meet join, invitations")
	add("external.guest_visibility", "What guests see in the directory", p.GuestVisibility, "applies to guest accounts", "Directory search")
	if l, err := g.Store.ListTrusts(ctx, orgID); err == nil {
		n := 0
		names := []string{}
		for _, t := range l {
			if t.Status == "active" && t.Direction == "outgoing" {
				n++
				names = append(names, t.PartnerOrgName)
			}
		}
		v := "none"
		if n > 0 {
			v = strings.Join(names, ", ")
		}
		add("external.trusts", "Trusted partner organizations", v, "people of these organizations skip the partner-domain list for the actions both sides allow (an explicit block still wins)", "Same sites as partner domains")
	}
	return out
}

// GuestInviteOpen reports whether the caller may create guest invitations
// without the user-admin permission (policy level lets plain people invite).
func (g *Gate) GuestInviteOpen(ctx context.Context) bool {
	orgID, _, ok := actor(ctx)
	if !ok {
		return false
	}
	return g.InviterAllowed(ctx, g.Store.Policy(ctx, orgID).GuestInvites)
}

// GuestVisible returns the people a guest may see in directory searches:
// themselves, their sponsor, people who shared something with them and members
// of the groups they belong to. restricted is false for non-guests and when the
// policy switches guest_visibility to "directory".
func (g *Gate) GuestVisible(ctx context.Context) (ids map[string]bool, restricted bool) {
	orgID, me, ok := actor(ctx)
	if !ok || me.Type != authz.TypeGuest {
		return nil, false
	}
	if g.Store.Policy(ctx, orgID).GuestVisibility == "directory" {
		return nil, false
	}
	ids = map[string]bool{me.ID: true}
	if g.Pool == nil {
		return ids, true
	}
	collect := func(q string, args ...any) {
		rows, err := g.Pool.Query(ctx, q, args...)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil && id != "" {
				ids[id] = true
			}
		}
	}
	collect(`SELECT sponsor_id::text FROM adrient.users WHERE id::text=$1 AND sponsor_id IS NOT NULL`, me.ID)
	collect(`SELECT granted_by::text FROM adrient.object_grants WHERE grantee_user_id::text=$1 AND granted_by IS NOT NULL`, me.ID)
	collect(`SELECT gm2.user_id::text FROM adrient.group_members gm1
JOIN adrient.group_members gm2 ON gm2.group_id = gm1.group_id WHERE gm1.user_id::text=$1`, me.ID)
	return ids, true
}
