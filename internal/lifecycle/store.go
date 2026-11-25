// Package lifecycle implements user types (member/external/guest/service),
// blocking and expiry, invitations with an accept flow, and the sign-in policy
// hooked into the OIDC callback (identity plan section B).
package lifecycle

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors.
var (
	ErrNotFound      = errors.New("not found")
	ErrInvalid       = errors.New("invalid request")
	ErrExists        = errors.New("a user with this email already belongs to the org")
	ErrInviteGone    = errors.New("invitation is invalid, expired, revoked or already used")
	ErrEmailMismatch = errors.New("signed-in email does not match the invitation")
	ErrOtherOrg      = errors.New("this account already belongs to another workspace")
)

// Defaults (plan O4).
const (
	DefaultGuestDays    = 30
	DefaultExternalDays = 90
	InviteValidity      = 7 * 24 * time.Hour
)

// ValidTypes are the types that can be assigned through the API.
var ValidTypes = map[string]bool{"member": true, "external": true, "guest": true, "service": true}

// Store is the lifecycle data layer.
type Store struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// UserRow is one row of the admin identity users list.
type UserRow struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	DisplayName   string     `json:"display_name"`
	UPN           string     `json:"upn"`
	UserType      string     `json:"user_type"`
	Status        string     `json:"status"`
	ExpiresAt     *time.Time `json:"expires_at"`
	SponsorID     string     `json:"sponsor_id"`
	SponsorEmail  string     `json:"sponsor_email"`
	BlockedReason string     `json:"blocked_reason"`
	LastSignInAt  *time.Time `json:"last_sign_in_at"`
	GuestApps     []string   `json:"guest_apps"`
	CreatedAt     time.Time  `json:"created_at"`
}

// UserFilter narrows ListUsers.
type UserFilter struct {
	Type, Status, Q string
	Limit           int
	Offset          int
}

// ListUsers returns the org's users with lifecycle fields.
func (s *Store) ListUsers(ctx context.Context, orgID string, f UserFilter) ([]UserRow, error) {
	if f.Limit <= 0 || f.Limit > 501 {
		f.Limit = 200
	}
	rows, err := s.Pool.Query(ctx, `
SELECT u.id::text, u.email, u.display_name, COALESCE(u.upn,''), u.user_type, u.status, u.expires_at,
       COALESCE(u.sponsor_id::text,''), COALESCE(sp.email,''), u.blocked_reason, u.last_sign_in_at, u.guest_apps, u.created_at
FROM adrient.users u LEFT JOIN adrient.users sp ON sp.id = u.sponsor_id
WHERE u.org_id = $1::uuid
  AND ($2 = '' OR u.user_type = $2)
  AND ($3 = '' OR u.status = $3 OR ($3 = 'blocked' AND u.status = 'expired'))
  AND ($4 = '' OR u.email ILIKE '%'||$4||'%' OR u.display_name ILIKE '%'||$4||'%')
ORDER BY lower(COALESCE(NULLIF(u.display_name,''), u.email)), u.id LIMIT $5 OFFSET $6`,
		orgID, f.Type, f.Status, f.Q, f.Limit, max(f.Offset, 0))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserRow{}
	for rows.Next() {
		var r UserRow
		if err := rows.Scan(&r.ID, &r.Email, &r.DisplayName, &r.UPN, &r.UserType, &r.Status, &r.ExpiresAt,
			&r.SponsorID, &r.SponsorEmail, &r.BlockedReason, &r.LastSignInAt, &r.GuestApps, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) revokeSessions(ctx context.Context, userID string) {
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.sessions SET revoked_at = now() WHERE user_id = $1::uuid AND revoked_at IS NULL`, userID)
}

// Block marks the user blocked (sign-in refused, sessions revoked). Returns the
// user's email for auditing.
func (s *Store) Block(ctx context.Context, orgID, userID, reason string) (string, error) {
	var email string
	err := s.Pool.QueryRow(ctx, `UPDATE adrient.users SET status='blocked', blocked_at=now(), blocked_reason=$3, updated_at=now()
		WHERE id=$2::uuid AND org_id=$1::uuid RETURNING email`, orgID, userID, reason).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	s.revokeSessions(ctx, userID)
	return email, nil
}

// Unblock re-activates a blocked user. A guest/external whose expiry already
// passed gets +30 days so the next expiry run does not re-block them at once.
func (s *Store) Unblock(ctx context.Context, orgID, userID string) (string, error) {
	var email string
	err := s.Pool.QueryRow(ctx, `UPDATE adrient.users SET status='active', blocked_at=NULL, blocked_reason='',
		  expires_at = CASE WHEN expires_at IS NOT NULL AND expires_at <= now() THEN now() + interval '30 days' ELSE expires_at END,
		  updated_at=now()
		WHERE id=$2::uuid AND org_id=$1::uuid RETURNING email`, orgID, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return email, err
}

// UserPatch edits type / expiry / sponsor / guest apps. ClearExpiry removes the expiry.
type UserPatch struct {
	UserType    *string    `json:"user_type"`
	ExpiresAt   *time.Time `json:"expires_at"`
	ClearExpiry bool       `json:"clear_expiry"`
	SponsorID   *string    `json:"sponsor_id"`
	GuestApps   *[]string  `json:"guest_apps"`
}

// UpdateUser applies a patch and returns before/after types for the audit row.
func (s *Store) UpdateUser(ctx context.Context, orgID, userID string, p UserPatch) (before, after string, err error) {
	if p.UserType != nil && !ValidTypes[*p.UserType] {
		return "", "", fmt.Errorf("%w: user_type", ErrInvalid)
	}
	err = s.Pool.QueryRow(ctx, `SELECT user_type FROM adrient.users WHERE id=$1::uuid AND org_id=$2::uuid`, userID, orgID).Scan(&before)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	after = before
	if p.UserType != nil {
		after = *p.UserType
		if _, err = s.Pool.Exec(ctx, `UPDATE adrient.users SET user_type=$2, updated_at=now() WHERE id=$1::uuid`, userID, after); err != nil {
			return before, after, err
		}
		// Becoming guest/external requires an expiry: default it.
		if after == "guest" || after == "external" {
			days := DefaultGuestDays
			if after == "external" {
				days = DefaultExternalDays
			}
			_, _ = s.Pool.Exec(ctx, `UPDATE adrient.users SET expires_at = COALESCE(expires_at, now() + make_interval(days => $2)) WHERE id=$1::uuid`, userID, days)
		}
		if after == "guest" {
			_, _ = s.Pool.Exec(ctx, `UPDATE adrient.users SET guest_apps = CASE WHEN cardinality(guest_apps)=0 THEN '{drive}' ELSE guest_apps END WHERE id=$1::uuid`, userID)
		}
	}
	if p.ClearExpiry && after != "guest" && after != "external" {
		_, err = s.Pool.Exec(ctx, `UPDATE adrient.users SET expires_at=NULL WHERE id=$1::uuid`, userID)
	} else if p.ExpiresAt != nil {
		_, err = s.Pool.Exec(ctx, `UPDATE adrient.users SET expires_at=$2 WHERE id=$1::uuid`, userID, *p.ExpiresAt)
	}
	if err != nil {
		return before, after, err
	}
	if p.SponsorID != nil {
		if *p.SponsorID != "" {
			var ok bool
			if err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE id=$1::uuid AND org_id=$2::uuid)`, *p.SponsorID, orgID).Scan(&ok); err != nil || !ok {
				return before, after, fmt.Errorf("%w: sponsor", ErrInvalid)
			}
		}
		if _, err = s.Pool.Exec(ctx, `UPDATE adrient.users SET sponsor_id=NULLIF($2,'')::uuid WHERE id=$1::uuid`, userID, *p.SponsorID); err != nil {
			return before, after, err
		}
	}
	if p.GuestApps != nil {
		if _, err = s.Pool.Exec(ctx, `UPDATE adrient.users SET guest_apps=$2 WHERE id=$1::uuid`, userID, *p.GuestApps); err != nil {
			return before, after, err
		}
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.users SET updated_at=now() WHERE id=$1::uuid`, userID)
	return before, after, nil
}

// ExpiredUser is one account the expiry job blocked.
type ExpiredUser struct{ ID, OrgID, Email string }

// ExpireDue blocks every active guest/external whose expires_at has passed,
// revokes their sessions and returns them.
func (s *Store) ExpireDue(ctx context.Context) ([]ExpiredUser, error) {
	rows, err := s.Pool.Query(ctx, `UPDATE adrient.users SET status='blocked', blocked_at=now(), blocked_reason='expired', updated_at=now()
		WHERE status='active' AND user_type IN ('guest','external') AND expires_at IS NOT NULL AND expires_at <= $1
		RETURNING id::text, org_id::text, email`, s.now())
	if err != nil {
		return nil, err
	}
	var out []ExpiredUser
	for rows.Next() {
		var e ExpiredUser
		if err := rows.Scan(&e.ID, &e.OrgID, &e.Email); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, e)
	}
	rows.Close()
	for _, e := range out {
		s.revokeSessions(ctx, e.ID)
	}
	return out, rows.Err()
}

// Expiring is an account nearing its expiry.
type Expiring struct {
	ID, OrgID, Email, SponsorEmail string
	ExpiresAt                      time.Time
	Kind                           string
}

// DueNotices returns accounts expiring within 14 / 3 days that have not yet been
// notified for that window, and marks them notified.
func (s *Store) DueNotices(ctx context.Context) ([]Expiring, error) {
	var out []Expiring
	for _, w := range []struct {
		kind string
		days int
	}{{"exp3", 3}, {"exp14", 14}} {
		rows, err := s.Pool.Query(ctx, `
SELECT u.id::text, u.org_id::text, u.email, COALESCE(sp.email,''), u.expires_at
FROM adrient.users u LEFT JOIN adrient.users sp ON sp.id = u.sponsor_id
WHERE u.status='active' AND u.user_type IN ('guest','external') AND u.expires_at > $1
  AND u.expires_at <= $1 + make_interval(days => $2)
  AND NOT EXISTS (SELECT 1 FROM adrient.lifecycle_notices n WHERE n.user_id = u.id AND n.kind = $3)`,
			s.now(), w.days, w.kind)
		if err != nil {
			return nil, err
		}
		var batch []Expiring
		for rows.Next() {
			e := Expiring{Kind: w.kind}
			if err := rows.Scan(&e.ID, &e.OrgID, &e.Email, &e.SponsorEmail, &e.ExpiresAt); err != nil {
				rows.Close()
				return nil, err
			}
			batch = append(batch, e)
		}
		rows.Close()
		for _, e := range batch {
			_, _ = s.Pool.Exec(ctx, `INSERT INTO adrient.lifecycle_notices (user_id, kind) VALUES ($1::uuid,$2) ON CONFLICT DO NOTHING`, e.ID, e.Kind)
			if w.kind == "exp3" { // the 3-day notice supersedes the 14-day one
				_, _ = s.Pool.Exec(ctx, `INSERT INTO adrient.lifecycle_notices (user_id, kind) VALUES ($1::uuid,'exp14') ON CONFLICT DO NOTHING`, e.ID)
			}
		}
		out = append(out, batch...)
	}
	return out, nil
}

// ---- invitations -----------------------------------------------------------

// Invitation is one adrient.invitations row (the secret is never exposed).
type Invitation struct {
	ID            string     `json:"id"`
	OrgID         string     `json:"org_id"`
	Email         string     `json:"email"`
	UserType      string     `json:"user_type"`
	InvitedBy     string     `json:"invited_by"`
	SponsorID     string     `json:"sponsor_id"`
	SponsorEmail  string     `json:"sponsor_email"`
	AppIDs        []string   `json:"app_ids"`
	UserExpiresAt *time.Time `json:"user_expires_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	AcceptedAt    *time.Time `json:"accepted_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	CreatedAt     time.Time  `json:"created_at"`
	State         string     `json:"state"` // pending | accepted | revoked | expired
}

// InviteInput is the create payload.
type InviteInput struct {
	Email      string   `json:"email"`
	UserType   string   `json:"user_type"`
	SponsorID  string   `json:"sponsor_id"`
	AppIDs     []string `json:"app_ids"`
	ExpireDays int      `json:"expire_days"`
}

const invCols = `i.id::text, i.org_id::text, i.email, i.user_type, COALESCE(i.invited_by::text,''), COALESCE(i.sponsor_id::text,''),
	COALESCE(sp.email,''), i.app_ids, i.user_expires_at, i.expires_at, i.accepted_at, i.revoked_at, i.created_at`

func (s *Store) scanInv(row pgx.Row) (Invitation, error) {
	var v Invitation
	err := row.Scan(&v.ID, &v.OrgID, &v.Email, &v.UserType, &v.InvitedBy, &v.SponsorID, &v.SponsorEmail, &v.AppIDs,
		&v.UserExpiresAt, &v.ExpiresAt, &v.AcceptedAt, &v.RevokedAt, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	v.State = "pending"
	switch {
	case v.AcceptedAt != nil:
		v.State = "accepted"
	case v.RevokedAt != nil:
		v.State = "revoked"
	case !v.ExpiresAt.After(s.now()):
		v.State = "expired"
	}
	return v, err
}

func newSecret() (secret string, hash []byte) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	secret = base64.RawURLEncoding.EncodeToString(b)
	return secret, hashSecret(secret)
}

func hashSecret(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// CreateInvitation validates and stores an invitation, returning it with the
// one-time secret (shown once; only its hash is stored).
func (s *Store) CreateInvitation(ctx context.Context, orgID, invitedBy string, in InviteInput) (Invitation, string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(in.Email))
	if err != nil || !strings.Contains(addr.Address, "@") {
		return Invitation{}, "", fmt.Errorf("%w: email", ErrInvalid)
	}
	email := strings.ToLower(addr.Address)
	if in.UserType == "" {
		in.UserType = "guest"
	}
	if in.UserType != "guest" && in.UserType != "external" && in.UserType != "member" {
		return Invitation{}, "", fmt.Errorf("%w: user_type", ErrInvalid)
	}
	var n int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.users WHERE org_id=$1::uuid AND lower(email)=$2`, orgID, email).Scan(&n)
	if n > 0 {
		return Invitation{}, "", ErrExists
	}
	sponsor := in.SponsorID
	if sponsor == "" {
		sponsor = invitedBy
	}
	var userExp *time.Time
	if in.UserType != "member" {
		days := in.ExpireDays
		if days <= 0 {
			days = DefaultGuestDays
			if in.UserType == "external" {
				days = DefaultExternalDays
			}
		}
		if days > 730 {
			days = 730
		}
		t := s.now().Add(time.Duration(days) * 24 * time.Hour)
		userExp = &t
	}
	apps := in.AppIDs
	if in.UserType == "guest" && len(apps) == 0 {
		apps = []string{"drive"}
	}
	if apps == nil {
		apps = []string{}
	}
	// Supersede earlier open invitations to the same address.
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.invitations SET revoked_at=now() WHERE org_id=$1::uuid AND lower(email)=$2 AND accepted_at IS NULL AND revoked_at IS NULL`, orgID, email)
	secret, hash := newSecret()
	var id string
	err = s.Pool.QueryRow(ctx, `INSERT INTO adrient.invitations (org_id, email, user_type, invited_by, sponsor_id, app_ids, user_expires_at, expires_at, secret_hash)
		VALUES ($1::uuid,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6,$7,$8,$9) RETURNING id::text`,
		orgID, email, in.UserType, invitedBy, sponsor, apps, userExp, s.now().Add(InviteValidity), hash).Scan(&id)
	if err != nil {
		return Invitation{}, "", err
	}
	inv, err := s.GetInvitation(ctx, orgID, id)
	return inv, secret, err
}

// GetInvitation returns one invitation of the org.
func (s *Store) GetInvitation(ctx context.Context, orgID, id string) (Invitation, error) {
	return s.scanInv(s.Pool.QueryRow(ctx, `SELECT `+invCols+` FROM adrient.invitations i
		LEFT JOIN adrient.users sp ON sp.id = i.sponsor_id WHERE i.org_id=$1::uuid AND i.id=$2::uuid`, orgID, id))
}

// ListInvitations returns the org's invitations, newest first.
func (s *Store) ListInvitations(ctx context.Context, orgID string) ([]Invitation, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+invCols+` FROM adrient.invitations i
		LEFT JOIN adrient.users sp ON sp.id = i.sponsor_id WHERE i.org_id=$1::uuid ORDER BY i.created_at DESC LIMIT 200`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invitation{}
	for rows.Next() {
		v, err := s.scanInv(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// RevokeInvitation cancels a pending invitation.
func (s *Store) RevokeInvitation(ctx context.Context, orgID, id string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE adrient.invitations SET revoked_at=now() WHERE org_id=$1::uuid AND id=$2::uuid AND accepted_at IS NULL AND revoked_at IS NULL`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ResendInvitation issues a fresh secret and validity window for a pending invitation.
func (s *Store) ResendInvitation(ctx context.Context, orgID, id string) (Invitation, string, error) {
	secret, hash := newSecret()
	tag, err := s.Pool.Exec(ctx, `UPDATE adrient.invitations SET secret_hash=$3, expires_at=$4
		WHERE org_id=$1::uuid AND id=$2::uuid AND accepted_at IS NULL AND revoked_at IS NULL`, orgID, id, hash, s.now().Add(InviteValidity))
	if err != nil {
		return Invitation{}, "", err
	}
	if tag.RowsAffected() == 0 {
		return Invitation{}, "", ErrNotFound
	}
	inv, err := s.GetInvitation(ctx, orgID, id)
	return inv, secret, err
}

// LookupPublic resolves a secret to a still-valid invitation plus the org name.
func (s *Store) LookupPublic(ctx context.Context, secret string) (Invitation, string, error) {
	var orgName string
	row := s.Pool.QueryRow(ctx, `SELECT `+invCols+`, o.display_name FROM adrient.invitations i
		JOIN adrient.orgs o ON o.id = i.org_id LEFT JOIN adrient.users sp ON sp.id = i.sponsor_id
		WHERE i.secret_hash = $1`, hashSecret(secret))
	var v Invitation
	err := row.Scan(&v.ID, &v.OrgID, &v.Email, &v.UserType, &v.InvitedBy, &v.SponsorID, &v.SponsorEmail, &v.AppIDs,
		&v.UserExpiresAt, &v.ExpiresAt, &v.AcceptedAt, &v.RevokedAt, &v.CreatedAt, &orgName)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, "", ErrInviteGone
	}
	if err != nil {
		return v, "", err
	}
	if v.AcceptedAt != nil || v.RevokedAt != nil || !v.ExpiresAt.After(s.now()) {
		return v, orgName, ErrInviteGone
	}
	return v, orgName, nil
}

// Accept redeems the invitation for the signed-in user (one time). The user's
// email must equal the invitation email and the user must already belong to the
// inviting org (the sign-in callback places invited e-mails there first).
func (s *Store) Accept(ctx context.Context, secret, userID, userEmail, userOrgID string) (Invitation, error) {
	// Idempotent for the redeeming user: the sign-in callback may already have
	// consumed the invitation for this very account.
	if prev, perr := s.scanInv(s.Pool.QueryRow(ctx, `SELECT `+invCols+` FROM adrient.invitations i
		LEFT JOIN adrient.users sp ON sp.id = i.sponsor_id
		WHERE i.secret_hash=$1 AND i.accepted_by=$2::uuid AND i.accepted_at IS NOT NULL`, hashSecret(secret), userID)); perr == nil {
		return prev, nil
	}
	inv, _, err := s.LookupPublic(ctx, secret)
	if err != nil {
		return inv, err
	}
	if !strings.EqualFold(strings.TrimSpace(userEmail), inv.Email) {
		return inv, ErrEmailMismatch
	}
	if userOrgID != inv.OrgID {
		return inv, ErrOtherOrg
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE adrient.invitations SET accepted_at=now(), accepted_by=$2::uuid
		WHERE id=$1::uuid AND accepted_at IS NULL AND revoked_at IS NULL`, inv.ID, userID)
	if err != nil {
		return inv, err
	}
	if tag.RowsAffected() == 0 {
		return inv, ErrInviteGone
	}
	if err := s.applyInvite(ctx, userID, inv); err != nil {
		return inv, err
	}
	return inv, nil
}

func (s *Store) applyInvite(ctx context.Context, userID string, inv Invitation) error {
	_, err := s.Pool.Exec(ctx, `UPDATE adrient.users SET user_type=$2, expires_at=$3, sponsor_id=NULLIF($4,'')::uuid, guest_apps=$5, updated_at=now()
		WHERE id=$1::uuid`, userID, inv.UserType, inv.UserExpiresAt, inv.SponsorID, inv.AppIDs)
	return err
}
