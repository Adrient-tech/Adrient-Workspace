package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/orgdomains"
	"github.com/Adrient-tech/Adrient-Workspace/internal/users"
)

// AuditFn records an identity event (audit.Recorder.Emit).
type AuditFn func(ctx context.Context, service, action string, detail map[string]any)

// Policy implements auth.SignInPolicy on top of the store and the domain store.
type Policy struct {
	Store   *Store
	Domains *orgdomains.Store
	Audit   AuditFn
	// SeatCheck refuses a first sign-in that would add a member beyond the plan's seats.
	SeatCheck func(ctx context.Context, orgID string) error
}

var _ auth.SignInPolicy = (*Policy)(nil)

func (p *Policy) emit(ctx context.Context, service, action string, d map[string]any) {
	if p.Audit != nil {
		p.Audit(ctx, service, action, d)
	}
}

// Route implements auth.SignInPolicy. It needs a verified e-mail claim.
func (p *Policy) Route(ctx context.Context, email string, emailVerified bool) (*auth.SignInRoute, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || !emailVerified {
		return nil, nil
	}
	// 1. A pending invitation for this address.
	var r auth.SignInRoute
	var sponsor string
	err := p.Store.Pool.QueryRow(ctx, `SELECT org_id::text, id::text, user_type, COALESCE(sponsor_id::text,''), user_expires_at, app_ids
		FROM adrient.invitations WHERE lower(email)=$1 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > $2
		ORDER BY created_at DESC LIMIT 1`, email, p.Store.now()).
		Scan(&r.OrgID, &r.InvitationID, &r.UserType, &sponsor, &r.ExpiresAt, &r.GuestApps)
	if err == nil {
		r.SponsorID = sponsor
		r.Via = "invitation"
		return &r, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	// 2. A verified domain with auto-join (org signup_mode = domain_auto_join).
	dom := orgdomains.EmailDomain(email)
	if dom == "" {
		return nil, nil
	}
	r = auth.SignInRoute{UserType: "member", Via: "domain_auto_join"}
	err = p.Store.Pool.QueryRow(ctx, `SELECT d.org_id::text, d.id::text FROM adrient.org_domains d
		JOIN adrient.org_settings s ON s.org_id = d.org_id
		WHERE d.domain=$1 AND d.status='verified' AND d.auto_join AND s.signup_mode='domain_auto_join'`, dom).Scan(&r.OrgID, &r.HomeDomainID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// SignupOpen implements auth.SignInPolicy.
func (p *Policy) SignupOpen(ctx context.Context, orgID string) bool {
	st, err := p.Domains.GetSettings(ctx, orgID)
	return err == nil && st.SignupMode != "invite_only"
}

// ErrDenied carries a machine-readable denial reason.
type ErrDenied struct{ Reason string }

func (e ErrDenied) Error() string {
	switch e.Reason {
	case "blocked":
		return "this account is blocked"
	case "expired":
		return "this account has expired"
	case "domain_not_allowed":
		return "your e-mail domain is not allowed to sign in to this workspace"
	case "no_seats":
		return "this workspace has no free seat; ask an admin to add seats"
	}
	return e.Reason
}

// Gate implements auth.SignInPolicy.
func (p *Policy) Gate(ctx context.Context, orgID, email, ip string, existing *users.User, route *auth.SignInRoute) error {
	deny := func(reason, utype string) error {
		p.emit(ctx, "identity", "signin.denied", map[string]any{"org_id": orgID, "actor_email": email, "reason": reason, "ip": ip, "user_type": utype})
		return ErrDenied{Reason: reason}
	}
	utype := "member"
	if existing != nil {
		utype = existing.EffectiveType()
		if existing.Blocked(p.Store.now()) {
			reason := "blocked"
			if existing.BlockedReason == "expired" || existing.Status == "expired" || (existing.ExpiresAt != nil && !existing.ExpiresAt.After(p.Store.now())) {
				reason = "expired"
			}
			return deny(reason, utype)
		}
	} else if route != nil && route.UserType != "" {
		utype = route.UserType
	}
	if utype != "member" {
		return nil // guests/externals are admitted by invitation, not by domain
	}
	if existing == nil && p.SeatCheck != nil {
		if err := p.SeatCheck(ctx, orgID); err != nil {
			return deny("no_seats", utype)
		}
	}
	st, err := p.Domains.GetSettings(ctx, orgID)
	if err != nil || len(st.AllowedSigninDomains) == 0 {
		return nil
	}
	dom := orgdomains.EmailDomain(email)
	for _, a := range st.AllowedSigninDomains {
		if a == dom {
			return nil
		}
	}
	if existing != nil {
		var admin bool
		_ = p.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.org_admins WHERE org_id=$1::uuid AND user_id=$2::uuid)
			OR EXISTS(SELECT 1 FROM adrient.role_assignments ra JOIN adrient.roles r ON r.id=ra.role_id
			          WHERE ra.org_id=$1::uuid AND ra.subject_type='user' AND ra.subject_id=$2::uuid AND r.key IN ('owner','admin'))`,
			orgID, existing.ID).Scan(&admin)
		if admin {
			return nil // admins are never locked out by the allow list
		}
	}
	return deny("domain_not_allowed", utype)
}

// Finish implements auth.SignInPolicy.
func (p *Policy) Finish(ctx context.Context, u users.User, route *auth.SignInRoute, ip string) error {
	pool := p.Store.Pool
	now := p.Store.now()
	if route != nil && route.Via == "invitation" {
		if _, err := pool.Exec(ctx, `UPDATE adrient.users SET user_type=$2, expires_at=$3, sponsor_id=NULLIF($4,'')::uuid, guest_apps=$5, updated_at=now()
			WHERE id=$1::uuid`, u.ID, route.UserType, route.ExpiresAt, route.SponsorID, route.GuestApps); err != nil {
			return err
		}
		_, _ = pool.Exec(ctx, `UPDATE adrient.invitations SET accepted_at=$2, accepted_by=$3::uuid WHERE id=$1::uuid AND accepted_at IS NULL`,
			route.InvitationID, now, u.ID)
		p.emit(ctx, "lifecycle", "user.accept", map[string]any{"org_id": u.OrgID, "actor_id": u.ID, "actor_email": u.Email,
			"resource_type": "invitation", "resource_id": route.InvitationID, "user_type": route.UserType, "ip": ip})
	} else if route != nil && route.Via == "domain_auto_join" {
		p.emit(ctx, "domain", "user.join", map[string]any{"org_id": u.OrgID, "actor_id": u.ID, "actor_email": u.Email,
			"resource_type": "domain", "resource_id": route.HomeDomainID, "via": "auto_join", "ip": ip})
	}
	// UPN and home domain.
	email := strings.ToLower(strings.TrimSpace(u.Email))
	if dom := orgdomains.EmailDomain(email); dom != "" {
		_, _ = pool.Exec(ctx, `UPDATE adrient.users SET home_domain_id = (SELECT id FROM adrient.org_domains
			WHERE org_id=$2::uuid AND domain=$3 AND status='verified') WHERE id=$1::uuid AND home_domain_id IS NULL`, u.ID, u.OrgID, dom)
	}
	if u.UPN == "" && email != "" {
		upn := email
		if primary := p.Domains.Primary(ctx, u.OrgID); primary != "" && (route == nil || route.Via != "invitation" || u.EffectiveType() == "member") {
			if local, _, ok := strings.Cut(email, "@"); ok && orgdomains.EmailDomain(email) == primary {
				upn = local + "@" + primary
			}
		}
		if _, err := pool.Exec(ctx, `UPDATE adrient.users SET upn=$2 WHERE id=$1::uuid AND upn IS NULL`, u.ID, upn); err != nil {
			_ = err // unique clash with another row: leave the UPN unset
		}
	}
	_, err := pool.Exec(ctx, `UPDATE adrient.users SET last_sign_in_at=$2 WHERE id=$1::uuid`, u.ID, now)
	utype := u.EffectiveType()
	if route != nil && route.UserType != "" {
		utype = route.UserType
	}
	p.emit(ctx, "identity", "signin.allowed", map[string]any{"org_id": u.OrgID, "actor_id": u.ID, "actor_email": u.Email,
		"user_type": utype, "ip": ip})
	return err
}

// RunExpiry is the lifecycle job: block expired guests/contractors, send the
// 14/3-day sponsor notices (audit rows + the optional Notify hook) and re-check
// domains. It returns how many accounts were blocked.
func (p *Policy) RunExpiry(ctx context.Context, notify func(ctx context.Context, to, subject, body string)) (int, error) {
	expired, err := p.Store.ExpireDue(ctx)
	if err != nil {
		return 0, err
	}
	for _, e := range expired {
		p.emit(ctx, "lifecycle", "user.expire", map[string]any{"org_id": e.OrgID, "resource_type": "user", "resource_id": e.ID, "email": e.Email, "reason": "expired"})
	}
	notices, err := p.Store.DueNotices(ctx)
	if err != nil {
		return len(expired), err
	}
	for _, n := range notices {
		p.emit(ctx, "lifecycle", "user.expiry_notice", map[string]any{"org_id": n.OrgID, "resource_type": "user", "resource_id": n.ID,
			"email": n.Email, "sponsor": n.SponsorEmail, "window": n.Kind, "expires_at": n.ExpiresAt.Format(time.RFC3339)})
		if notify != nil && n.SponsorEmail != "" {
			notify(ctx, n.SponsorEmail, "Guest access expires soon: "+n.Email,
				fmt.Sprintf("%s loses access on %s. Extend it from Admin > Users if still needed.", n.Email, n.ExpiresAt.Format("2 Jan 2006")))
		}
	}
	if p.Domains != nil {
		_, _ = p.Domains.CheckAllDue(ctx, 24*time.Hour)
	}
	return len(expired), nil
}
