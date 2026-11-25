// Package orgdomains owns verified org domains (TXT challenge), the org sign-up
// settings, UPN assignment and the sign-in routing decisions that depend on the
// email domain (identity plan section A).
package orgdomains

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned by the Store.
var (
	ErrInvalidDomain = errors.New("invalid domain")
	ErrFreeMail      = errors.New("free-mail domains cannot be claimed by an org")
	ErrClaimed       = errors.New("domain is already claimed by an org")
	ErrNotFound      = errors.New("domain not found")
	ErrNotVerified   = errors.New("domain is not verified")
	ErrPrimary       = errors.New("cannot remove the primary domain")
	ErrInUse         = errors.New("domain still has users; reassign first")
)

// ChallengePrefix is the DNS label under the domain that holds the TXT value.
const ChallengePrefix = "_adrient-challenge."

// Status values.
const (
	StatusPending  = "pending"
	StatusVerified = "verified"
	StatusFailed   = "failed"
	StatusRemoved  = "removed"
)

// Domain is one adrient.org_domains row.
type Domain struct {
	ID             string     `json:"id"`
	OrgID          string     `json:"org_id"`
	Domain         string     `json:"domain"`
	Status         string     `json:"status"`
	VerifyHost     string     `json:"verify_host"`
	VerifyTXT      string     `json:"verify_txt"`
	VerifiedAt     *time.Time `json:"verified_at,omitempty"`
	LastCheckedAt  *time.Time `json:"last_checked_at,omitempty"`
	IsPrimary      bool       `json:"is_primary"`
	Purpose        []string   `json:"purpose"`
	AllowSignup    bool       `json:"allow_signup"`
	AutoJoin       bool       `json:"auto_join"`
	RequireApprove bool       `json:"require_approval"`
	CreatedAt      time.Time  `json:"created_at"`
	UserCount      int        `json:"user_count"`
}

// Resolver looks up TXT records. The default implementation queries public
// resolvers; tests inject a fake.
type Resolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// Store reads and writes domains and org settings.
type Store struct {
	Pool     *pgxpool.Pool
	Resolver Resolver
	Now      func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

var labelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// freeMail lists consumer mail domains that can never be an org domain.
var freeMail = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "outlook.com": true, "hotmail.com": true, "live.com": true,
	"yahoo.com": true, "ymail.com": true, "icloud.com": true, "me.com": true, "aol.com": true,
	"proton.me": true, "protonmail.com": true, "gmx.com": true, "gmx.net": true, "mail.com": true,
	"zoho.com": true, "yandex.com": true, "msn.com": true, "qq.com": true, "163.com": true,
}

// IsFreeMail reports whether domain is a consumer mail provider.
func IsFreeMail(domain string) bool { return freeMail[strings.ToLower(domain)] }

// Normalize lowercases and validates a domain name (ASCII/punycode only).
func Normalize(d string) (string, error) {
	d = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(d), ".")))
	if d == "" || len(d) > 253 || !strings.Contains(d, ".") {
		return "", ErrInvalidDomain
	}
	for _, l := range strings.Split(d, ".") {
		if !labelRe.MatchString(l) {
			return "", ErrInvalidDomain
		}
	}
	return d, nil
}

// EmailDomain returns the lower-case domain part of an email, or "".
func EmailDomain(email string) string {
	i := strings.LastIndexByte(email, '@')
	if i < 0 || i == len(email)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[i+1:]))
}

func newTXT() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "adrient-verify=" + hex.EncodeToString(b)
}

const domCols = `d.id::text, d.org_id::text, d.domain, d.status, d.verify_txt, d.verified_at, d.last_checked_at,
	d.is_primary, d.purpose, d.allow_signup, d.auto_join, d.require_approval, d.created_at,
	(SELECT count(*) FROM adrient.users u WHERE u.home_domain_id = d.id)`

func scanDomain(row pgx.Row) (Domain, error) {
	var d Domain
	err := row.Scan(&d.ID, &d.OrgID, &d.Domain, &d.Status, &d.VerifyTXT, &d.VerifiedAt, &d.LastCheckedAt,
		&d.IsPrimary, &d.Purpose, &d.AllowSignup, &d.AutoJoin, &d.RequireApprove, &d.CreatedAt, &d.UserCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	d.VerifyHost = ChallengePrefix + d.Domain
	return d, err
}

// List returns the org's non-removed domains, primary first.
func (s *Store) List(ctx context.Context, orgID string) ([]Domain, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+domCols+` FROM adrient.org_domains d
		WHERE d.org_id = $1 AND d.status <> 'removed' ORDER BY d.is_primary DESC, d.created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Domain{}
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Get returns one domain of the org.
func (s *Store) Get(ctx context.Context, orgID, id string) (Domain, error) {
	return scanDomain(s.Pool.QueryRow(ctx, `SELECT `+domCols+` FROM adrient.org_domains d
		WHERE d.org_id = $1 AND d.id = $2::uuid AND d.status <> 'removed'`, orgID, id))
}

// Add claims a domain for the org in state pending and returns it with the TXT
// value to publish. A domain claimed (pending or verified) by any org is refused.
func (s *Store) Add(ctx context.Context, orgID, domain string) (Domain, error) {
	d, err := Normalize(domain)
	if err != nil {
		return Domain{}, err
	}
	if IsFreeMail(d) {
		return Domain{}, ErrFreeMail
	}
	var id string
	err = s.Pool.QueryRow(ctx, `INSERT INTO adrient.org_domains (org_id, domain, verify_txt)
		VALUES ($1::uuid, $2, $3) RETURNING id::text`, orgID, d, newTXT()).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "org_domains_claim_uq") {
			return Domain{}, ErrClaimed
		}
		return Domain{}, err
	}
	return s.Get(ctx, orgID, id)
}

// Verify checks the TXT challenge now and moves the domain to verified (or
// failed when it was verified/failed before and the record is gone). The first
// verified domain of an org becomes its primary domain.
func (s *Store) Verify(ctx context.Context, orgID, id string) (Domain, bool, error) {
	d, err := s.Get(ctx, orgID, id)
	if err != nil {
		return d, false, err
	}
	ok := s.checkTXT(ctx, d)
	now := s.now()
	switch {
	case ok:
		_, err = s.Pool.Exec(ctx, `UPDATE adrient.org_domains SET status='verified',
			verified_at = COALESCE(verified_at, $2), last_checked_at = $2 WHERE id = $1::uuid`, id, now)
		if err == nil {
			// First verified domain becomes primary.
			_, err = s.Pool.Exec(ctx, `UPDATE adrient.org_domains SET is_primary = true
				WHERE id = $1::uuid AND NOT EXISTS (SELECT 1 FROM adrient.org_domains p WHERE p.org_id = $2::uuid AND p.is_primary)`, id, orgID)
		}
	case d.Status == StatusVerified && d.VerifiedAt != nil:
		// Verified earlier: keep verified until it has failed for 14 days.
		next := StatusVerified
		if now.Sub(lastGood(d)) > 14*24*time.Hour {
			next = StatusFailed
		}
		_, err = s.Pool.Exec(ctx, `UPDATE adrient.org_domains SET status=$2, last_checked_at=$3,
			allow_signup = CASE WHEN $2='failed' THEN false ELSE allow_signup END WHERE id = $1::uuid`, id, next, now)
	default:
		_, err = s.Pool.Exec(ctx, `UPDATE adrient.org_domains SET last_checked_at=$2 WHERE id = $1::uuid`, id, now)
	}
	if err != nil {
		return d, ok, err
	}
	d, err = s.Get(ctx, orgID, id)
	return d, ok, err
}

func lastGood(d Domain) time.Time {
	if d.VerifiedAt != nil {
		return *d.VerifiedAt
	}
	return d.CreatedAt
}

func (s *Store) checkTXT(ctx context.Context, d Domain) bool {
	if s.Resolver == nil {
		return false
	}
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	recs, err := s.Resolver.LookupTXT(cctx, ChallengePrefix+d.Domain)
	if err != nil {
		return false
	}
	for _, r := range recs {
		if strings.TrimSpace(r) == d.VerifyTXT {
			return true
		}
	}
	return false
}

// Patch holds the editable flags; nil fields are unchanged.
type Patch struct {
	Primary        *bool     `json:"is_primary"`
	AllowSignup    *bool     `json:"allow_signup"`
	AutoJoin       *bool     `json:"auto_join"`
	RequireApprove *bool     `json:"require_approval"`
	Purpose        *[]string `json:"purpose"`
}

// Update applies a patch. Only verified domains may be primary / allow sign-up / auto-join.
func (s *Store) Update(ctx context.Context, orgID, id string, p Patch) (Domain, error) {
	d, err := s.Get(ctx, orgID, id)
	if err != nil {
		return d, err
	}
	wantsPower := (p.Primary != nil && *p.Primary) || (p.AllowSignup != nil && *p.AllowSignup) || (p.AutoJoin != nil && *p.AutoJoin)
	if wantsPower && d.Status != StatusVerified {
		return d, ErrNotVerified
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return d, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if p.Primary != nil && *p.Primary {
		if _, err = tx.Exec(ctx, `UPDATE adrient.org_domains SET is_primary=false WHERE org_id=$1::uuid AND is_primary`, orgID); err != nil {
			return d, err
		}
		if _, err = tx.Exec(ctx, `UPDATE adrient.org_domains SET is_primary=true WHERE id=$1::uuid`, id); err != nil {
			return d, err
		}
	}
	if p.AllowSignup != nil {
		if _, err = tx.Exec(ctx, `UPDATE adrient.org_domains SET allow_signup=$2 WHERE id=$1::uuid`, id, *p.AllowSignup); err != nil {
			return d, err
		}
	}
	if p.AutoJoin != nil {
		if _, err = tx.Exec(ctx, `UPDATE adrient.org_domains SET auto_join=$2 WHERE id=$1::uuid`, id, *p.AutoJoin); err != nil {
			return d, err
		}
	}
	if p.RequireApprove != nil {
		if _, err = tx.Exec(ctx, `UPDATE adrient.org_domains SET require_approval=$2 WHERE id=$1::uuid`, id, *p.RequireApprove); err != nil {
			return d, err
		}
	}
	if p.Purpose != nil {
		if _, err = tx.Exec(ctx, `UPDATE adrient.org_domains SET purpose=$2 WHERE id=$1::uuid`, id, *p.Purpose); err != nil {
			return d, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return d, err
	}
	return s.Get(ctx, orgID, id)
}

// Remove deletes a domain claim. The primary domain and domains with users are refused.
func (s *Store) Remove(ctx context.Context, orgID, id string) error {
	d, err := s.Get(ctx, orgID, id)
	if err != nil {
		return err
	}
	if d.IsPrimary {
		return ErrPrimary
	}
	if d.UserCount > 0 {
		return ErrInUse
	}
	_, err = s.Pool.Exec(ctx, `UPDATE adrient.org_domains SET status='removed', is_primary=false, auto_join=false, allow_signup=false
		WHERE id=$1::uuid`, id)
	return err
}

// Primary returns the org's primary verified domain name, or "".
func (s *Store) Primary(ctx context.Context, orgID string) string {
	var d string
	_ = s.Pool.QueryRow(ctx, `SELECT domain FROM adrient.org_domains
		WHERE org_id=$1::uuid AND is_primary AND status='verified'`, orgID).Scan(&d)
	return d
}

// Settings are the org sign-up / sign-in settings.
type Settings struct {
	SignupMode           string   `json:"signup_mode"`
	AllowedSigninDomains []string `json:"allowed_signin_domains"`
	PersonalOrgsAllowed  bool     `json:"personal_orgs_allowed"`
}

// SignupModes lists the valid signup_mode values.
var SignupModes = map[string]bool{"invite_only": true, "domain_auto_join": true, "domain_approval": true, "open": true}

// GetSettings returns the org's settings (defaults when no row exists).
func (s *Store) GetSettings(ctx context.Context, orgID string) (Settings, error) {
	st := Settings{SignupMode: "invite_only", AllowedSigninDomains: []string{}, PersonalOrgsAllowed: true}
	err := s.Pool.QueryRow(ctx, `SELECT signup_mode, allowed_signin_domains, personal_orgs_allowed
		FROM adrient.org_settings WHERE org_id=$1::uuid`, orgID).Scan(&st.SignupMode, &st.AllowedSigninDomains, &st.PersonalOrgsAllowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, nil
	}
	return st, err
}

// PutSettings validates and stores the settings.
func (s *Store) PutSettings(ctx context.Context, orgID string, st Settings) (Settings, error) {
	if !SignupModes[st.SignupMode] {
		return st, fmt.Errorf("invalid signup_mode %q", st.SignupMode)
	}
	clean := []string{}
	for _, d := range st.AllowedSigninDomains {
		n, err := Normalize(d)
		if err != nil {
			return st, fmt.Errorf("invalid allowed domain %q", d)
		}
		clean = append(clean, n)
	}
	st.AllowedSigninDomains = clean
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.org_settings (org_id, signup_mode, allowed_signin_domains, personal_orgs_allowed, updated_at)
		VALUES ($1::uuid,$2,$3,$4, now())
		ON CONFLICT (org_id) DO UPDATE SET signup_mode=EXCLUDED.signup_mode,
		  allowed_signin_domains=EXCLUDED.allowed_signin_domains,
		  personal_orgs_allowed=EXCLUDED.personal_orgs_allowed, updated_at=now()`,
		orgID, st.SignupMode, st.AllowedSigninDomains, st.PersonalOrgsAllowed)
	return st, err
}

// DomainClaimedBy returns whether a verified domain with this name exists (no org data).
func (s *Store) DomainClaimed(ctx context.Context, domain string) bool {
	var n int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM adrient.org_domains WHERE domain=$1 AND status='verified'`, strings.ToLower(domain)).Scan(&n)
	return n > 0
}

// CheckAllDue re-verifies every non-removed domain whose last check is older than
// age. Used by the daily job.
func (s *Store) CheckAllDue(ctx context.Context, age time.Duration) (int, error) {
	rows, err := s.Pool.Query(ctx, `SELECT org_id::text, id::text FROM adrient.org_domains
		WHERE status IN ('pending','verified') AND (last_checked_at IS NULL OR last_checked_at < $1)`, s.now().Add(-age))
	if err != nil {
		return 0, err
	}
	type pair struct{ o, i string }
	var ps []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.o, &p.i); err != nil {
			rows.Close()
			return 0, err
		}
		ps = append(ps, p)
	}
	rows.Close()
	for _, p := range ps {
		_, _, _ = s.Verify(ctx, p.o, p.i)
	}
	return len(ps), nil
}

// NetResolver queries the system resolver plus two public ones.
type NetResolver struct{}

// LookupTXT implements Resolver.
func (NetResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	var lastErr error
	for _, srv := range []string{"1.1.1.1:53", "8.8.8.8:53"} {
		srv := srv
		r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "udp", srv)
		}}
		recs, err := r.LookupTXT(ctx, name)
		if err == nil && len(recs) > 0 {
			return recs, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
