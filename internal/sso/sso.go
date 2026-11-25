// Package sso lets an organization sign in with Microsoft Entra ID, Google,
// another OIDC provider or SAML. The providers are created in Zitadel (the
// identity provider of this stack) through its admin API. Adrient adds domain
// routing (an e-mail domain goes straight to its provider), just-in-time user
// creation and the "require SSO for this domain" rule.
package sso

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/zitadelmgmt"
)

// Errors with admin-safe messages.
var (
	ErrBad      = errors.New("invalid request")
	ErrNotFound = errors.New("connection not found")
)

// Service holds the dependencies.
type Service struct {
	Pool *pgxpool.Pool
	ZM   *zitadelmgmt.Client
	// Audit records an admin audit event.
	Audit func(ctx context.Context, orgID, actorID, action string, detail map[string]any)
}

// Connection is a configured provider (secrets are never returned).
type Connection struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	IdPID     string   `json:"zitadel_idp_id"`
	Domains   []string `json:"domains"`
	JIT       bool     `json:"jit"`
	Enforce   bool     `json:"enforce"`
	Enabled   bool     `json:"enabled"`
	Issuer    string   `json:"issuer,omitempty"`
	TenantID  string   `json:"tenant_id,omitempty"`
	ClientID  string   `json:"client_id,omitempty"`
	CreatedAt string   `json:"created_at"`
}

// CreateInput is what the admin enters.
type CreateInput struct {
	Kind         string   `json:"kind"` // entra | google | oidc | saml
	Name         string   `json:"name"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Issuer       string   `json:"issuer"`
	TenantID     string   `json:"tenant_id"`
	MetadataXML  string   `json:"metadata_xml"`
	MetadataURL  string   `json:"metadata_url"`
	Domains      []string `json:"domains"`
	JIT          bool     `json:"jit"`
	Enforce      bool     `json:"enforce"`
}

func (s *Service) audit(ctx context.Context, org, actor, action string, d map[string]any) {
	if s.Audit != nil {
		s.Audit(ctx, org, actor, action, d)
	}
}

func normDomains(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(d), "@")))
		if d == "" {
			continue
		}
		if !strings.Contains(d, ".") || strings.ContainsAny(d, " /\\:@") || len(d) > 253 {
			return nil, fmt.Errorf("%w: %q is not a domain", ErrBad, d)
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out, nil
}

// verifiedDomains checks that every domain is a verified domain of the org.
func (s *Service) verifiedDomains(ctx context.Context, tx pgx.Tx, orgID string, domains []string) error {
	for _, d := range domains {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.org_domains WHERE org_id::text=$1 AND domain=$2 AND status='verified')`, orgID, d).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: %s is not a verified domain of this organization. Verify it under Domains first", ErrBad, d)
		}
	}
	return nil
}

func (s *Service) applyJIT(ctx context.Context, tx pgx.Tx, orgID string, domains []string, jit bool) error {
	for _, d := range domains {
		if _, err := tx.Exec(ctx, `UPDATE adrient.org_domains SET auto_join=$3 WHERE org_id::text=$1 AND domain=$2`, orgID, d, jit); err != nil {
			return err
		}
	}
	if jit && len(domains) > 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.org_settings (org_id, signup_mode) VALUES ($1,'domain_auto_join')
ON CONFLICT (org_id) DO UPDATE SET signup_mode='domain_auto_join', updated_at=now() WHERE adrient.org_settings.signup_mode='invite_only'`, orgID); err != nil {
			return err
		}
	}
	return nil
}

// Create configures the provider in Zitadel and stores the connection.
func (s *Service) Create(ctx context.Context, orgID, actor string, in CreateInput) (Connection, error) {
	if s.ZM == nil {
		return Connection{}, fmt.Errorf("%w: Zitadel is not connected to this server (set ADRIENT_ZITADEL_API_URL and ADRIENT_ZITADEL_SERVICE_TOKEN). See docs/sso-scim.md for the manual steps", ErrBad)
	}
	domains, err := normDomains(in.Domains)
	if err != nil {
		return Connection{}, err
	}
	if len(domains) == 0 {
		return Connection{}, fmt.Errorf("%w: add at least one e-mail domain to route to this provider", ErrBad)
	}
	if in.Enforce && !in.JIT {
		// Enforcing SSO without creating accounts is allowed: people must exist first.
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Connection{}, err
	}
	defer tx.Rollback(ctx)
	if err := s.verifiedDomains(ctx, tx, orgID, domains); err != nil {
		return Connection{}, err
	}
	for _, d := range domains {
		var taken bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.sso_domains WHERE domain=$1)`, d).Scan(&taken)
		if taken {
			return Connection{}, fmt.Errorf("%w: %s already routes to another provider", ErrBad, d)
		}
	}
	zin := zitadelmgmt.IdPInput{Kind: zitadelmgmt.IdPKind(in.Kind), Name: strings.TrimSpace(in.Name), ClientID: in.ClientID,
		ClientSecret: in.ClientSecret, Issuer: strings.TrimSpace(in.Issuer), TenantID: strings.TrimSpace(in.TenantID),
		MetadataXML: in.MetadataXML, MetadataURL: strings.TrimSpace(in.MetadataURL), AutoCreate: in.JIT}
	if err := zin.Validate(); err != nil {
		return Connection{}, fmt.Errorf("%w: %v", ErrBad, err)
	}
	idpID, err := s.ZM.CreateIdP(ctx, zin)
	if err != nil {
		return Connection{}, fmt.Errorf("%w: Zitadel refused the provider: %v", ErrBad, err)
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO adrient.sso_connections (org_id, kind, name, zitadel_idp_id, jit, enforce, issuer, tenant_id, client_id, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::uuid) RETURNING id::text`,
		orgID, in.Kind, zin.Name, idpID, in.JIT, in.Enforce, zin.Issuer, zin.TenantID, in.ClientID, actor).Scan(&id)
	if err == nil {
		for _, d := range domains {
			if _, err = tx.Exec(ctx, `INSERT INTO adrient.sso_domains (domain, connection_id, org_id) VALUES ($1,$2::uuid,$3)`, d, id, orgID); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = s.applyJIT(ctx, tx, orgID, domains, in.JIT)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		_ = s.ZM.DeleteIdP(ctx, idpID) // do not leave an orphan provider in Zitadel
		return Connection{}, err
	}
	s.audit(ctx, orgID, actor, "sso.create", map[string]any{"kind": in.Kind, "name": zin.Name, "domains": domains, "jit": in.JIT, "enforce": in.Enforce})
	return s.Get(ctx, orgID, id)
}

const connCols = `c.id::text, c.kind, c.name, c.zitadel_idp_id, COALESCE((SELECT array_agg(d.domain ORDER BY d.domain) FROM adrient.sso_domains d WHERE d.connection_id=c.id), '{}'),
c.jit, c.enforce, c.enabled, c.issuer, c.tenant_id, c.client_id, c.created_at::text`

func scanConn(row pgx.Row) (Connection, error) {
	var c Connection
	err := row.Scan(&c.ID, &c.Kind, &c.Name, &c.IdPID, &c.Domains, &c.JIT, &c.Enforce, &c.Enabled, &c.Issuer, &c.TenantID, &c.ClientID, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// Get returns one connection of the org.
func (s *Service) Get(ctx context.Context, orgID, id string) (Connection, error) {
	return scanConn(s.Pool.QueryRow(ctx, `SELECT `+connCols+` FROM adrient.sso_connections c WHERE c.org_id::text=$1 AND c.id::text=$2`, orgID, id))
}

// List returns the org's connections.
func (s *Service) List(ctx context.Context, orgID string) ([]Connection, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+connCols+` FROM adrient.sso_connections c WHERE c.org_id::text=$1 ORDER BY c.created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		c, err := scanConn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateInput changes routing and enforcement of an existing connection.
type UpdateInput struct {
	Domains []string `json:"domains"`
	JIT     *bool    `json:"jit"`
	Enforce *bool    `json:"enforce"`
	Enabled *bool    `json:"enabled"`
}

// Update edits a connection.
func (s *Service) Update(ctx context.Context, orgID, actor, id string, in UpdateInput) (Connection, error) {
	cur, err := s.Get(ctx, orgID, id)
	if err != nil {
		return cur, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return cur, err
	}
	defer tx.Rollback(ctx)
	domains := cur.Domains
	if in.Domains != nil {
		if domains, err = normDomains(in.Domains); err != nil {
			return cur, err
		}
		if len(domains) == 0 {
			return cur, fmt.Errorf("%w: keep at least one domain", ErrBad)
		}
		if err := s.verifiedDomains(ctx, tx, orgID, domains); err != nil {
			return cur, err
		}
		for _, d := range domains {
			var other bool
			_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.sso_domains WHERE domain=$1 AND connection_id::text<>$2)`, d, id).Scan(&other)
			if other {
				return cur, fmt.Errorf("%w: %s already routes to another provider", ErrBad, d)
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM adrient.sso_domains WHERE connection_id::text=$1`, id); err != nil {
			return cur, err
		}
		for _, d := range domains {
			if _, err := tx.Exec(ctx, `INSERT INTO adrient.sso_domains (domain, connection_id, org_id) VALUES ($1,$2::uuid,$3)`, d, id, orgID); err != nil {
				return cur, err
			}
		}
	}
	jit, enforce, enabled := cur.JIT, cur.Enforce, cur.Enabled
	if in.JIT != nil {
		jit = *in.JIT
	}
	if in.Enforce != nil {
		enforce = *in.Enforce
	}
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.sso_connections SET jit=$3, enforce=$4, enabled=$5 WHERE org_id::text=$1 AND id::text=$2`, orgID, id, jit, enforce, enabled); err != nil {
		return cur, err
	}
	if err := s.applyJIT(ctx, tx, orgID, domains, jit); err != nil {
		return cur, err
	}
	if err := tx.Commit(ctx); err != nil {
		return cur, err
	}
	s.audit(ctx, orgID, actor, "sso.update", map[string]any{"connection": cur.Name, "domains": domains, "jit": jit, "enforce": enforce, "enabled": enabled})
	return s.Get(ctx, orgID, id)
}

// Delete removes the connection and the provider from Zitadel.
func (s *Service) Delete(ctx context.Context, orgID, actor, id string) error {
	cur, err := s.Get(ctx, orgID, id)
	if err != nil {
		return err
	}
	if s.ZM != nil {
		if err := s.ZM.DeleteIdP(ctx, cur.IdPID); err != nil {
			var ze *zitadelmgmt.Error
			if !(errors.As(err, &ze) && ze.Status == 404) {
				return fmt.Errorf("could not remove the provider from Zitadel: %w", err)
			}
		}
	}
	if _, err := s.Pool.Exec(ctx, `DELETE FROM adrient.sso_connections WHERE org_id::text=$1 AND id::text=$2`, orgID, id); err != nil {
		return err
	}
	s.audit(ctx, orgID, actor, "sso.delete", map[string]any{"connection": cur.Name})
	return nil
}

// Discover finds the enabled connection that serves an e-mail address.
func (s *Service) Discover(ctx context.Context, email string) (Connection, bool) {
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return Connection{}, false
	}
	dom := strings.ToLower(strings.TrimSpace(email[at+1:]))
	c, err := scanConn(s.Pool.QueryRow(ctx, `SELECT `+connCols+` FROM adrient.sso_connections c JOIN adrient.sso_domains d ON d.connection_id=c.id
WHERE d.domain=$1 AND c.enabled`, dom))
	return c, err == nil
}

// PasswordGate refuses a password sign-in for an address whose domain requires
// SSO. Organization admins keep password access so a broken provider cannot
// lock everybody out.
func (s *Service) PasswordGate(ctx context.Context, email string) error {
	c, ok := s.Discover(ctx, email)
	if !ok || !c.Enforce {
		return nil
	}
	var admin bool
	_ = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users u WHERE lower(u.email)=lower($1) AND (
EXISTS(SELECT 1 FROM adrient.org_admins a WHERE a.user_id=u.id) OR EXISTS(SELECT 1 FROM adrient.role_assignments ra JOIN adrient.roles r ON r.id=ra.role_id
WHERE ra.subject_type='user' AND ra.subject_id=u.id AND r.key IN ('owner','admin'))))`, email).Scan(&admin)
	if admin {
		return nil
	}
	return fmt.Errorf("Your organization requires single sign-on. Use \"Sign in with SSO\" (%s).", c.Name)
}
