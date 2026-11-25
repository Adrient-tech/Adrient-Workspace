package authz

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGLoader reads assignments from adrient.role_assignments.
type PGLoader struct{ Pool *pgxpool.Pool }

// Load returns the caller's assignments: direct user assignments, group
// assignments (groups.member_ids contains the user), the legacy org_admins
// grant as Admin, and the implicit Member role unless the caller is a Guest.
func (l PGLoader) Load(ctx context.Context, orgID, userID string) ([]Assignment, error) {
	rows, err := l.Pool.Query(ctx, `
SELECT r.key, ra.scope_app,
       COALESCE((SELECT array_agg(rp.permission) FROM adrient.role_permissions rp WHERE rp.role_id = r.id), '{}')
FROM adrient.role_assignments ra
JOIN adrient.roles r ON r.id = ra.role_id
WHERE ra.org_id = $1
  AND ( (ra.subject_type = 'user' AND ra.subject_id = $2::uuid)
     OR (ra.subject_type = 'group' AND ra.subject_id IN (
            SELECT g.id FROM adrient.groups g WHERE g.org_id = $1 AND g.member_ids ? ($2::uuid)::text)))`,
		orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("authz.Load assignments: %w", err)
	}
	var out []Assignment
	for rows.Next() {
		var a Assignment
		if err := rows.Scan(&a.RoleKey, &a.ScopeApp, &a.Permissions); err != nil {
			rows.Close()
			return nil, fmt.Errorf("authz.Load scan: %w", err)
		}
		out = append(out, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	have := map[string]bool{}
	for _, a := range out {
		have[a.RoleKey] = true
	}
	if !have[RoleAdmin] {
		var legacy bool
		if err := l.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM adrient.org_admins WHERE org_id=$1 AND user_id=$2::uuid)`,
			orgID, userID).Scan(&legacy); err != nil {
			return nil, fmt.Errorf("authz.Load legacy admin: %w", err)
		}
		if legacy {
			out = append(out, Assignment{RoleKey: RoleAdmin, Permissions: []string{"*"}})
		}
	}
	// A user of type guest is a Guest regardless of role rows: no implicit Member.
	var utype string
	_ = l.Pool.QueryRow(ctx, `SELECT user_type FROM adrient.users WHERE id=$1::uuid`, userID).Scan(&utype)
	if utype == TypeGuest && !have[RoleGuest] {
		out = append(out, Assignment{RoleKey: RoleGuest})
		have[RoleGuest] = true
	}
	if !have[RoleGuest] {
		var perms []string
		if err := l.Pool.QueryRow(ctx, `
SELECT COALESCE(array_agg(rp.permission), '{}')
FROM adrient.roles r JOIN adrient.role_permissions rp ON rp.role_id = r.id
WHERE r.key = 'member' AND r.org_id IS NULL`).Scan(&perms); err != nil {
			return nil, fmt.Errorf("authz.Load member perms: %w", err)
		}
		out = append(out, Assignment{RoleKey: RoleMember, Permissions: perms})
	}
	return out, nil
}
