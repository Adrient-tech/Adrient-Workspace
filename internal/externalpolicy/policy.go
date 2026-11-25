// Package externalpolicy decides whether an organization may collaborate with an
// outside person: invite, share, chat or meet. One function, Check, is the only
// rule; every share dialog and enforcement site calls it so the UI and the
// server cannot drift (plan C).
package externalpolicy

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Actions passed to Check.
const (
	ActionInvite = "invite"
	ActionShare  = "share"
	ActionChat   = "chat"
	ActionMeet   = "meet"
)

// Domain modes.
const (
	ModeAllowAll  = "allow_all"
	ModeAllowlist = "allowlist"
	ModeBlocklist = "blocklist"
)

// Policy is adrient.external_policy.
type Policy struct {
	GuestInvites                    string `json:"guest_invites"`
	DomainMode                      string `json:"domain_mode"`
	GuestVisibility                 string `json:"guest_visibility"`
	GuestDefaultExpiryDays          int    `json:"guest_default_expiry_days"`
	ExternalMemberDefaultExpiryDays int    `json:"external_member_default_expiry_days"`
	RequireMFAForGuests             bool   `json:"require_mfa_for_guests"`
}

// Default is the policy of an org that has not configured anything.
func Default() Policy {
	return Policy{GuestInvites: "admins_and_sponsors", DomainMode: ModeAllowAll, GuestVisibility: "limited",
		GuestDefaultExpiryDays: 30, ExternalMemberDefaultExpiryDays: 90, RequireMFAForGuests: true}
}

// Partner is one partner domain row.
type Partner struct {
	Domain    string         `json:"domain"`
	Effect    string         `json:"effect"` // allow | block
	Note      string         `json:"note"`
	Overrides map[string]any `json:"overrides,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// Verdict is the answer of Check.
type Verdict struct {
	Allowed  bool   `json:"allowed"`
	Status   string `json:"status"` // internal | allowed | blocked
	Domain   string `json:"domain"`
	Partner  bool   `json:"partner"`
	Reason   string `json:"reason"`
	External bool   `json:"external"`
}

// FreeMail lists consumer mail providers (allowed only when the mode is
// allow_all or the domain is explicitly allow-listed).
var FreeMail = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "outlook.com": true, "hotmail.com": true, "live.com": true,
	"yahoo.com": true, "icloud.com": true, "me.com": true, "aol.com": true, "proton.me": true,
	"protonmail.com": true, "gmx.com": true, "mail.com": true, "yandex.com": true, "zoho.com": true,
}

// EmailDomain returns the lower-case domain of an address, or "".
func EmailDomain(email string) string {
	if a, err := mail.ParseAddress(strings.TrimSpace(email)); err == nil {
		email = a.Address
	}
	i := strings.LastIndexByte(email, '@')
	if i < 0 || i == len(email)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[i+1:]))
}

// Evaluate is the pure rule. internal lists the org's verified domains.
//
//	internal domain          -> allowed (internal)
//	partner effect=block     -> blocked, in every mode
//	mode allow_all           -> allowed
//	mode allowlist           -> allowed only when the domain is allow-listed
//	mode blocklist           -> allowed unless blocked; free-mail needs an allow entry
func Evaluate(p Policy, partners []Partner, internal []string, email string) Verdict {
	d := EmailDomain(email)
	v := Verdict{Domain: d}
	if d == "" {
		v.Status, v.Reason = "blocked", "That is not a valid e-mail address."
		return v
	}
	for _, x := range internal {
		if strings.EqualFold(x, d) {
			v.Allowed, v.Status, v.Reason = true, "internal", "Same organization."
			return v
		}
	}
	v.External = true
	effect := ""
	for _, pr := range partners {
		if strings.EqualFold(pr.Domain, d) {
			effect, v.Partner = pr.Effect, true
		}
	}
	block := func(why string) Verdict { v.Allowed, v.Status, v.Reason = false, "blocked", why; return v }
	allow := func(why string) Verdict { v.Allowed, v.Status, v.Reason = true, "allowed", why; return v }
	switch {
	case effect == "block":
		return block(d + " is blocked by your organization's external collaboration policy.")
	case p.DomainMode == ModeAllowlist:
		if effect == "allow" {
			return allow(d + " is an allowed partner domain.")
		}
		return block(d + " is not on your organization's list of allowed partner domains.")
	case p.DomainMode == ModeBlocklist:
		if FreeMail[d] && effect != "allow" {
			return block("Personal mail domains such as " + d + " are not allowed unless an admin allows them.")
		}
		return allow("Not on the blocked list.")
	}
	return allow("External collaboration is open.")
}

// Store loads the policy, partners and internal domains for an org.
type Store struct {
	Pool *pgxpool.Pool

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	p        Policy
	partners []Partner
	internal []string
	exp      time.Time
}

// Invalidate drops cached data (call after any edit).
func (s *Store) Invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.mu.Unlock()
}

func (s *Store) load(ctx context.Context, orgID string) (cached, error) {
	s.mu.Lock()
	if c, ok := s.cache[orgID]; ok && time.Now().Before(c.exp) {
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()
	c := cached{p: Default(), exp: time.Now().Add(5 * time.Second)}
	p, err := s.Get(ctx, orgID)
	if err != nil {
		return c, err
	}
	c.p = p
	if c.partners, err = s.Partners(ctx, orgID); err != nil {
		return c, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT domain FROM adrient.org_domains WHERE org_id=$1 AND status='verified'`, orgID)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		if rows.Scan(&d) == nil {
			c.internal = append(c.internal, d)
		}
	}
	s.mu.Lock()
	if s.cache == nil {
		s.cache = map[string]cached{}
	}
	s.cache[orgID] = c
	s.mu.Unlock()
	return c, rows.Err()
}

// Check is the single entry point used by every enforcement site: may orgID
// do action with recipientEmail? It fails closed on a read error.
func (s *Store) Check(ctx context.Context, orgID, recipientEmail, action string) Verdict {
	c, err := s.load(ctx, orgID)
	if err != nil {
		return Verdict{Allowed: false, Status: "blocked", Domain: EmailDomain(recipientEmail), Reason: "Policy could not be read; try again."}
	}
	v := Evaluate(c.p, c.partners, c.internal, recipientEmail)
	return s.withTrust(ctx, c, orgID, recipientEmail, action, v)
}

// withTrust lets an active two-sided cross-org trust override a domain-mode
// refusal. An explicit block of the domain always wins.
func (s *Store) withTrust(ctx context.Context, c cached, orgID, email, action string, v Verdict) Verdict {
	if v.Allowed || s.isBlocked(c, email) {
		return v
	}
	if ok, name := s.TrustAllows(ctx, orgID, email, action); ok {
		v.Allowed, v.Status, v.Partner, v.Reason = true, "allowed", true, "Trusted partner organization "+name+"."
	}
	return v
}

// Policy returns the org's policy (defaults when unset or unreadable).
func (s *Store) Policy(ctx context.Context, orgID string) Policy {
	c, _ := s.load(ctx, orgID)
	return c.p
}

// ErrInvalid reports a bad policy value.
var ErrInvalid = errors.New("invalid external policy")

// Validate checks enumerations and ranges.
func (p Policy) Validate() error {
	ok := func(v string, l ...string) bool {
		for _, x := range l {
			if v == x {
				return true
			}
		}
		return false
	}
	if !ok(p.GuestInvites, "none", "admins", "admins_and_sponsors", "members", "everyone") ||
		!ok(p.DomainMode, ModeAllowAll, ModeAllowlist, ModeBlocklist) ||
		!ok(p.GuestVisibility, "limited", "directory") ||
		p.GuestDefaultExpiryDays < 1 || p.GuestDefaultExpiryDays > 730 ||
		p.ExternalMemberDefaultExpiryDays < 1 || p.ExternalMemberDefaultExpiryDays > 730 {
		return ErrInvalid
	}
	return nil
}

// Get reads the stored policy (defaults when no row).
func (s *Store) Get(ctx context.Context, orgID string) (Policy, error) {
	p := Default()
	err := s.Pool.QueryRow(ctx, `SELECT guest_invites, domain_mode, guest_visibility, guest_default_expiry_days,
external_member_default_expiry_days, require_mfa_for_guests FROM adrient.external_policy WHERE org_id=$1`, orgID).
		Scan(&p.GuestInvites, &p.DomainMode, &p.GuestVisibility, &p.GuestDefaultExpiryDays, &p.ExternalMemberDefaultExpiryDays, &p.RequireMFAForGuests)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Default(), err
	}
	return p, nil
}

// Put stores the policy.
func (s *Store) Put(ctx context.Context, orgID string, p Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO adrient.external_policy
(org_id, guest_invites, domain_mode, guest_visibility, guest_default_expiry_days, external_member_default_expiry_days, require_mfa_for_guests, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,now())
ON CONFLICT (org_id) DO UPDATE SET guest_invites=$2, domain_mode=$3, guest_visibility=$4,
guest_default_expiry_days=$5, external_member_default_expiry_days=$6, require_mfa_for_guests=$7, updated_at=now()`,
		orgID, p.GuestInvites, p.DomainMode, p.GuestVisibility, p.GuestDefaultExpiryDays, p.ExternalMemberDefaultExpiryDays, p.RequireMFAForGuests)
	s.Invalidate()
	return err
}

// Partners lists partner domains.
func (s *Store) Partners(ctx context.Context, orgID string) ([]Partner, error) {
	rows, err := s.Pool.Query(ctx, `SELECT domain, effect, note, overrides, created_at FROM adrient.partner_domains WHERE org_id=$1 ORDER BY domain`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Partner{}
	for rows.Next() {
		var p Partner
		if err := rows.Scan(&p.Domain, &p.Effect, &p.Note, &p.Overrides, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// NormalizeDomain lowercases and sanity-checks a partner domain.
func NormalizeDomain(d string) (string, error) {
	d = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(d), "@")))
	if d == "" || len(d) > 253 || !strings.Contains(d, ".") || strings.ContainsAny(d, " /\\:@") {
		return "", ErrInvalid
	}
	return d, nil
}

// PutPartner upserts a partner domain.
func (s *Store) PutPartner(ctx context.Context, orgID, actorID string, p Partner) error {
	d, err := NormalizeDomain(p.Domain)
	if err != nil || (p.Effect != "allow" && p.Effect != "block") {
		return ErrInvalid
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO adrient.partner_domains (org_id, domain, effect, note, created_by)
VALUES ($1,$2,$3,$4,$5::uuid) ON CONFLICT (org_id, domain) DO UPDATE SET effect=$3, note=$4`,
		orgID, d, p.Effect, p.Note, nullIfEmpty(actorID))
	s.Invalidate()
	return err
}

// DeletePartner removes a partner domain; it reports whether a row existed.
func (s *Store) DeletePartner(ctx context.Context, orgID, domain string) (bool, error) {
	d, err := NormalizeDomain(domain)
	if err != nil {
		return false, ErrInvalid
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM adrient.partner_domains WHERE org_id=$1 AND domain=$2`, orgID, d)
	s.Invalidate()
	return tag.RowsAffected() > 0, err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
