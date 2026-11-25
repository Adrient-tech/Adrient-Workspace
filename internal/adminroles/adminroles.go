// Package adminroles manages delegated admin roles (user admin, billing admin,
// help desk, read-only), their optional department scope, and admin API tokens.
// Only a super admin (Owner/Admin) can use it.
package adminroles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Adrient-tech/Adrient-Workspace/internal/apitokens"
	"github.com/Adrient-tech/Adrient-Workspace/internal/auth"
	"github.com/Adrient-tech/Adrient-Workspace/internal/authz"
)

// Role is a delegated admin role as shown in the admin.
type Role struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

var descriptions = map[string]string{
	"user_admin":     "Creates and manages people, groups, invitations, sign-in sessions, offboarding and access reviews.",
	"billing_admin":  "Manages seats, plans and licences and reads the user list and audit log.",
	"help_desk":      "Looks people up, resets passwords and signs people out. Cannot change roles or delete anyone.",
	"readonly_admin": "Reads every admin page and the audit log. Cannot change anything.",
}

// Assignment is one person holding a delegated role.
type Assignment struct {
	ID          string   `json:"id"`
	UserID      string   `json:"user_id"`
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	RoleKey     string   `json:"role_key"`
	Departments []string `json:"departments"`
}

// Token is an admin API token.
type Token struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	RoleKey   string     `json:"role_key"`
	Prefix    string     `json:"prefix"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at"`
}

var ErrBad = errors.New("invalid request")

// Service holds the dependencies.
type Service struct {
	Pool   *pgxpool.Pool
	Tokens *apitokens.Repository
	Roles  *authz.Service
	Audit  func(ctx context.Context, orgID, actorID, action string, detail map[string]any)
	Alert  func(ctx context.Context, orgID, event, subject, body string)
}

func (s *Service) audit(ctx context.Context, org, actor, action string, d map[string]any) {
	if s.Audit != nil {
		s.Audit(ctx, org, actor, action, d)
	}
}

// ListRoles returns the delegated roles and their permissions.
func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.Pool.Query(ctx, `SELECT r.key, r.name, COALESCE(array_agg(rp.permission ORDER BY rp.permission) FILTER (WHERE rp.permission IS NOT NULL), '{}')
FROM adrient.roles r LEFT JOIN adrient.role_permissions rp ON rp.role_id=r.id
WHERE r.org_id IS NULL AND r.key IN ('user_admin','billing_admin','help_desk','readonly_admin') GROUP BY r.key, r.name ORDER BY r.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Role{}
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.Key, &r.Name, &r.Permissions); err != nil {
			return nil, err
		}
		r.Description = descriptions[r.Key]
		out = append(out, r)
	}
	return out, rows.Err()
}

func delegable(key string) bool {
	switch key {
	case "user_admin", "billing_admin", "help_desk", "readonly_admin":
		return true
	}
	return false
}

// Assign gives a person a delegated role, optionally limited to departments.
func (s *Service) Assign(ctx context.Context, orgID, actor, userID, roleKey string, deptIDs []string) error {
	if !delegable(roleKey) {
		return fmt.Errorf("%w: unknown delegated role", ErrBad)
	}
	var ok bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE org_id::text=$1 AND id::text=$2 AND user_type='member' AND status='active' AND deleted_at IS NULL)`, orgID, userID).Scan(&ok); err != nil || !ok {
		return fmt.Errorf("%w: choose an active member of this organization", ErrBad)
	}
	for _, d := range deptIDs {
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.departments WHERE org_id::text=$1 AND id::text=$2)`, orgID, d).Scan(&ok); err != nil || !ok {
			return fmt.Errorf("%w: unknown department", ErrBad)
		}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.role_assignments (org_id, subject_type, subject_id, role_id, created_by)
SELECT $1, 'user', $2::uuid, r.id, NULLIF($4,'')::uuid FROM adrient.roles r WHERE r.org_id IS NULL AND r.key=$3 ON CONFLICT DO NOTHING`, orgID, userID, roleKey, actor); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.admin_scopes WHERE org_id::text=$1 AND user_id::text=$2`, orgID, userID); err != nil {
		return err
	}
	for _, d := range deptIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.admin_scopes (org_id, user_id, department_id) VALUES ($1,$2::uuid,$3::uuid)`, orgID, userID, d); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.audit(ctx, orgID, actor, "admin.role.assign", map[string]any{"user_id": userID, "role": roleKey, "departments": deptIDs})
	if s.Alert != nil {
		s.Alert(ctx, orgID, "admin.role", "Admin role assigned", "A delegated admin role ("+roleKey+") was assigned.")
	}
	return nil
}

// Unassign removes a delegated role (and the scope when it was the last one).
func (s *Service) Unassign(ctx context.Context, orgID, actor, assignmentID string) error {
	var userID, role string
	err := s.Pool.QueryRow(ctx, `DELETE FROM adrient.role_assignments ra USING adrient.roles r WHERE r.id=ra.role_id AND ra.org_id::text=$1 AND ra.id::text=$2
AND r.key IN ('user_admin','billing_admin','help_desk','readonly_admin') RETURNING ra.subject_id::text, r.key`, orgID, assignmentID).Scan(&userID, &role)
	if err != nil {
		return fmt.Errorf("%w: assignment not found", ErrBad)
	}
	_, _ = s.Pool.Exec(ctx, `DELETE FROM adrient.admin_scopes s WHERE s.org_id::text=$1 AND s.user_id::text=$2 AND NOT EXISTS(
SELECT 1 FROM adrient.role_assignments ra JOIN adrient.roles r ON r.id=ra.role_id WHERE ra.subject_id::text=$2 AND r.key IN ('user_admin','billing_admin','help_desk','readonly_admin'))`, orgID, userID)
	s.audit(ctx, orgID, actor, "admin.role.unassign", map[string]any{"user_id": userID, "role": role})
	return nil
}

// ListAssignments lists the people holding delegated roles.
func (s *Service) ListAssignments(ctx context.Context, orgID string) ([]Assignment, error) {
	rows, err := s.Pool.Query(ctx, `SELECT ra.id::text, u.id::text, u.email, u.display_name, r.key,
COALESCE((SELECT array_agg(d.name ORDER BY d.name) FROM adrient.admin_scopes sc JOIN adrient.departments d ON d.id=sc.department_id WHERE sc.user_id=u.id), '{}')
FROM adrient.role_assignments ra JOIN adrient.roles r ON r.id=ra.role_id JOIN adrient.users u ON u.id=ra.subject_id
WHERE ra.org_id::text=$1 AND ra.subject_type='user' AND r.key IN ('user_admin','billing_admin','help_desk','readonly_admin') AND u.user_type<>'service'
ORDER BY u.email, r.key`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Assignment{}
	for rows.Next() {
		var a Assignment
		if err := rows.Scan(&a.ID, &a.UserID, &a.Email, &a.Name, &a.RoleKey, &a.Departments); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ScopeAllows is authz.DelegatedScope: an admin without scope rows may act on
// anyone; a scoped admin only on people in their departments or sub-departments.
func (s *Service) ScopeAllows(ctx context.Context, orgID, adminUserID, target string) bool {
	var scoped bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.admin_scopes WHERE org_id::text=$1 AND user_id::text=$2)`, orgID, adminUserID).Scan(&scoped); err != nil {
		return false
	}
	if !scoped {
		return true
	}
	var ok bool
	err := s.Pool.QueryRow(ctx, `WITH RECURSIVE scope AS (
  SELECT department_id AS id FROM adrient.admin_scopes WHERE org_id::text=$1 AND user_id::text=$2
  UNION SELECT d.id FROM adrient.departments d JOIN scope s ON d.parent_id=s.id)
SELECT EXISTS(SELECT 1 FROM adrient.users u WHERE u.org_id::text=$1 AND (u.id::text=$3 OR u.oidc_subject=$3) AND u.department_id IN (SELECT id FROM scope))`,
		orgID, adminUserID, target).Scan(&ok)
	return err == nil && ok
}

// ---- admin API tokens ------------------------------------------------------------

// CreateToken makes a service account with a delegated role and a bearer token
// limited to the admin API. The plaintext is returned once.
func (s *Service) CreateToken(ctx context.Context, orgID, actor, name, roleKey string, expireDays int) (string, Token, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 || !delegable(roleKey) {
		return "", Token{}, fmt.Errorf("%w: a name and a delegated role are required", ErrBad)
	}
	if s.Tokens == nil {
		return "", Token{}, fmt.Errorf("%w: API tokens are not available", ErrBad)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", Token{}, err
	}
	defer tx.Rollback(ctx)
	var svc string
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.users (org_id, oidc_issuer, oidc_subject, email, display_name, user_type)
VALUES ($1,'adrient:service',gen_random_uuid()::text,'token-'||substr(gen_random_uuid()::text,1,8)||'@service.invalid',$2,'service') RETURNING id::text`, orgID, "Admin token: "+name).Scan(&svc); err != nil {
		return "", Token{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO adrient.role_assignments (org_id, subject_type, subject_id, role_id, created_by)
SELECT $1,'user',$2::uuid,r.id,NULLIF($4,'')::uuid FROM adrient.roles r WHERE r.org_id IS NULL AND r.key=$3`, orgID, svc, roleKey, actor); err != nil {
		return "", Token{}, err
	}
	scopes := []string{"admin"}
	if roleKey == "readonly_admin" {
		scopes = []string{"admin:read"}
	}
	var exp *time.Time
	if expireDays > 0 {
		t := time.Now().AddDate(0, 0, expireDays)
		exp = &t
	}
	plain, tok, err := s.Tokens.Create(ctx, svc, orgID, name, scopes, exp)
	if err != nil {
		return "", Token{}, err
	}
	var t Token
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.admin_api_tokens (org_id, name, role_key, service_user_id, api_token_id, prefix, created_by)
VALUES ($1,$2,$3,$4::uuid,$5::uuid,$6,NULLIF($7,'')::uuid) RETURNING id::text, name, role_key, prefix, created_at, revoked_at`,
		orgID, name, roleKey, svc, tok.ID, tok.Prefix, actor).Scan(&t.ID, &t.Name, &t.RoleKey, &t.Prefix, &t.CreatedAt, &t.RevokedAt); err != nil {
		return "", Token{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", Token{}, err
	}
	s.audit(ctx, orgID, actor, "admin.token.create", map[string]any{"name": name, "role": roleKey})
	return plain, t, nil
}

// ListTokens lists the org's admin tokens.
func (s *Service) ListTokens(ctx context.Context, orgID string) ([]Token, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, name, role_key, prefix, created_at, revoked_at FROM adrient.admin_api_tokens WHERE org_id::text=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Token{}
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.Name, &t.RoleKey, &t.Prefix, &t.CreatedAt, &t.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken revokes the token, removes the role and blocks the service account.
func (s *Service) RevokeToken(ctx context.Context, orgID, actor, id string) error {
	var svc, apiTok string
	err := s.Pool.QueryRow(ctx, `UPDATE adrient.admin_api_tokens SET revoked_at=now() WHERE org_id::text=$1 AND id::text=$2 AND revoked_at IS NULL
RETURNING service_user_id::text, COALESCE(api_token_id::text,'')`, orgID, id).Scan(&svc, &apiTok)
	if err != nil {
		return fmt.Errorf("%w: token not found", ErrBad)
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.api_tokens SET revoked_at=now() WHERE id::text=$1`, apiTok)
	_, _ = s.Pool.Exec(ctx, `DELETE FROM adrient.role_assignments WHERE subject_id::text=$1`, svc)
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.users SET status='blocked', blocked_at=now(), blocked_reason='Token revoked' WHERE id::text=$1`, svc)
	s.audit(ctx, orgID, actor, "admin.token.revoke", map[string]any{"token_id": id})
	return nil
}

// ---- HTTP --------------------------------------------------------------------------

// Paths.
const (
	RolesPath  = "/api/v1/admin/admin-roles"
	TokensPath = "/api/v1/admin/admin-tokens"
)

// Handler serves the roles and tokens API (super admin only).
type Handler struct{ Svc *Service }

// Match reports whether path belongs to this handler.
func Match(path string) bool {
	p := strings.TrimRight(path, "/")
	return p == RolesPath || strings.HasPrefix(p, RolesPath+"/") || p == TokensPath || strings.HasPrefix(p, TokensPath+"/")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	code, msg := http.StatusInternalServerError, "request failed"
	if errors.Is(err, ErrBad) {
		code, msg = http.StatusBadRequest, err.Error()
	}
	writeJSON(w, code, map[string]any{"error": msg})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o, ok := auth.OrgFromContext(r.Context())
	u, ok2 := auth.UserFromContext(r.Context())
	if !ok || !ok2 {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	// Super admins only: a delegated admin must never be able to widen their own rights.
	if !h.Svc.Roles.Can(r.Context(), authz.PermOrgAdmin) {
		authz.WriteForbidden(w, authz.PermOrgAdmin)
		return
	}
	ctx := r.Context()
	p := strings.TrimRight(r.URL.Path, "/")
	body := func(v any) bool { return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v) == nil }
	switch {
	case p == RolesPath && r.Method == http.MethodGet:
		roles, err := h.Svc.ListRoles(ctx)
		if err != nil {
			fail(w, err)
			return
		}
		as, err := h.Svc.ListAssignments(ctx, o.ID)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"roles": roles, "assignments": as})
	case p == RolesPath && r.Method == http.MethodPost:
		var b struct {
			UserID      string   `json:"user_id"`
			RoleKey     string   `json:"role_key"`
			Departments []string `json:"department_ids"`
		}
		if !body(&b) {
			fail(w, ErrBad)
			return
		}
		if err := h.Svc.Assign(ctx, o.ID, u.ID, b.UserID, b.RoleKey, b.Departments); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case strings.HasPrefix(p, RolesPath+"/") && r.Method == http.MethodDelete:
		if err := h.Svc.Unassign(ctx, o.ID, u.ID, strings.TrimPrefix(p, RolesPath+"/")); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case p == TokensPath && r.Method == http.MethodGet:
		l, err := h.Svc.ListTokens(ctx, o.ID)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tokens": l})
	case p == TokensPath && r.Method == http.MethodPost:
		var b struct {
			Name       string `json:"name"`
			RoleKey    string `json:"role_key"`
			ExpireDays int    `json:"expire_days"`
		}
		if !body(&b) {
			fail(w, ErrBad)
			return
		}
		plain, t, err := h.Svc.CreateToken(ctx, o.ID, u.ID, b.Name, b.RoleKey, b.ExpireDays)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"token": plain, "meta": t, "note": "Copy the token now. It is not shown again."})
	case strings.HasPrefix(p, TokensPath+"/") && r.Method == http.MethodDelete:
		if err := h.Svc.RevokeToken(ctx, o.ID, u.ID, strings.TrimPrefix(p, TokensPath+"/")); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	}
}
