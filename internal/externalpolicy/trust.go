package externalpolicy

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Trust is one side of a cross-organization trust (adrient.cross_org_trusts).
// A trust is active only when both organizations have an active row; either
// side may revoke, which revokes both rows.
type Trust struct {
	PartnerOrgID   string          `json:"partner_org_id"`
	PartnerOrgName string          `json:"partner_org_name"`
	Status         string          `json:"status"` // pending | active | revoked
	Direction      string          `json:"direction"`
	Inbound        map[string]bool `json:"inbound"`
	Outbound       map[string]bool `json:"outbound"`
	CreatedAt      time.Time       `json:"created_at"`
}

// Trust actions: what the trust lets people of the two orgs do together.
var TrustActions = []string{ActionShare, ActionChat, ActionMeet, ActionInvite}

var (
	ErrNoTrust       = errors.New("no such trust")
	ErrTrustSelf     = errors.New("an organization cannot trust itself")
	ErrTrustNotFound = errors.New("partner organization not found")
)

func cleanActions(m map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, a := range TrustActions {
		v, set := m[a]
		out[a] = !set || v // an action that is not mentioned is allowed
	}
	return out
}

// ProposeTrust opens (or reopens) a pending trust from orgID to the org that
// owns the verified domain (or has the slug) partnerRef.
func (s *Store) ProposeTrust(ctx context.Context, orgID, partnerRef string, outbound, inbound map[string]bool) (Trust, error) {
	partnerRef = strings.ToLower(strings.TrimSpace(partnerRef))
	var pid, pname string
	err := s.Pool.QueryRow(ctx, `SELECT o.id::text, o.display_name FROM adrient.orgs o
WHERE lower(o.slug)=$1 OR o.id IN (SELECT org_id FROM adrient.org_domains WHERE lower(domain)=$1 AND status='verified') LIMIT 1`, partnerRef).Scan(&pid, &pname)
	if errors.Is(err, pgx.ErrNoRows) {
		return Trust{}, ErrTrustNotFound
	}
	if err != nil {
		return Trust{}, err
	}
	if pid == orgID {
		return Trust{}, ErrTrustSelf
	}
	ob, ib := cleanActions(outbound), cleanActions(inbound)
	_, err = s.Pool.Exec(ctx, `INSERT INTO adrient.cross_org_trusts (org_id, partner_org_id, status, inbound, outbound)
VALUES ($1,$2,'pending',$3,$4)
ON CONFLICT (org_id, partner_org_id) DO UPDATE SET status='pending', inbound=$3, outbound=$4`, orgID, pid, ib, ob)
	s.Invalidate()
	return Trust{PartnerOrgID: pid, PartnerOrgName: pname, Status: "pending", Direction: "outgoing", Inbound: ib, Outbound: ob, CreatedAt: time.Now()}, err
}

// AcceptTrust makes the pending trust proposed by partnerOrgID active, creating
// orgID's own row with its own inbound/outbound choices.
func (s *Store) AcceptTrust(ctx context.Context, orgID, partnerOrgID string, outbound, inbound map[string]bool) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE adrient.cross_org_trusts SET status='active' WHERE org_id=$1 AND partner_org_id=$2 AND status='pending'`, partnerOrgID, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTrust
	}
	if _, err = tx.Exec(ctx, `INSERT INTO adrient.cross_org_trusts (org_id, partner_org_id, status, inbound, outbound)
VALUES ($1,$2,'active',$3,$4) ON CONFLICT (org_id, partner_org_id) DO UPDATE SET status='active', inbound=$3, outbound=$4`,
		orgID, partnerOrgID, cleanActions(inbound), cleanActions(outbound)); err != nil {
		return err
	}
	s.Invalidate()
	return tx.Commit(ctx)
}

// RevokeTrust revokes both rows between the two orgs (also declines a proposal).
func (s *Store) RevokeTrust(ctx context.Context, orgID, partnerOrgID string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE adrient.cross_org_trusts SET status='revoked'
WHERE (org_id=$1 AND partner_org_id=$2) OR (org_id=$2 AND partner_org_id=$1)`, orgID, partnerOrgID)
	s.Invalidate()
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNoTrust
	}
	return err
}

// ListTrusts returns the org's outgoing rows and the incoming proposals.
func (s *Store) ListTrusts(ctx context.Context, orgID string) ([]Trust, error) {
	rows, err := s.Pool.Query(ctx, `SELECT t.partner_org_id::text, o.display_name, t.status, 'outgoing', t.inbound, t.outbound, t.created_at
FROM adrient.cross_org_trusts t JOIN adrient.orgs o ON o.id=t.partner_org_id WHERE t.org_id=$1 AND t.status<>'revoked'
UNION ALL
SELECT t.org_id::text, o.display_name, t.status, 'incoming', t.inbound, t.outbound, t.created_at
FROM adrient.cross_org_trusts t JOIN adrient.orgs o ON o.id=t.org_id
WHERE t.partner_org_id=$1 AND t.status='pending'
ORDER BY 2`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Trust{}
	for rows.Next() {
		var t Trust
		if err := rows.Scan(&t.PartnerOrgID, &t.PartnerOrgName, &t.Status, &t.Direction, &t.Inbound, &t.Outbound, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TrustAllows reports whether orgID has an active two-sided trust with the
// organization that owns email's domain (or the user with that address) for
// action: our outbound and the partner's inbound must both allow it.
func (s *Store) TrustAllows(ctx context.Context, orgID, email, action string) (bool, string) {
	d := EmailDomain(email)
	if d == "" {
		return false, ""
	}
	var name string
	var ob, ib map[string]bool
	err := s.Pool.QueryRow(ctx, `SELECT o.display_name, a.outbound, b.inbound
FROM adrient.cross_org_trusts a
JOIN adrient.cross_org_trusts b ON b.org_id=a.partner_org_id AND b.partner_org_id=a.org_id
JOIN adrient.orgs o ON o.id=a.partner_org_id
WHERE a.org_id=$1 AND a.status='active' AND b.status='active'
  AND (a.partner_org_id IN (SELECT org_id FROM adrient.org_domains WHERE lower(domain)=$2 AND status='verified')
    OR a.partner_org_id IN (SELECT org_id FROM adrient.users WHERE lower(email)=lower($3)))
LIMIT 1`, orgID, d, email).Scan(&name, &ob, &ib)
	if err != nil {
		return false, ""
	}
	return ob[action] && ib[action], name
}

func (s *Store) isBlocked(c cached, email string) bool {
	d := EmailDomain(email)
	for _, p := range c.partners {
		if strings.EqualFold(p.Domain, d) && p.Effect == "block" {
			return true
		}
	}
	return false
}
