package users

import (
	"context"
	"fmt"
	"strings"
)

// SubjectPage is one keyset page of org members' identity-provider ids.
type SubjectPage struct {
	Subjects []string
	// NextKey and NextID continue after the last row; both empty on the last page.
	NextKey string
	NextID  string
}

// ListOIDCSubjectsPage returns up to limit members of orgID (ordered by name,
// then id) for the given issuer, after the (afterKey, afterID) cursor. The
// query q matches name, email, employee id and UPN. Backed by the
// users_org_name_key_idx index, so a page costs the same at row 5 or 50,000.
func (r *Repository) ListOIDCSubjectsPage(ctx context.Context, orgID, issuer, q, afterKey, afterID string, limit int) (SubjectPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	args := []any{orgID, issuer}
	where := "org_id = $1 AND oidc_issuer = $2 AND oidc_subject <> ''"
	if q = strings.TrimSpace(q); q != "" {
		args = append(args, "%"+strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)+"%")
		n := fmt.Sprint(len(args))
		where += " AND (display_name ILIKE $" + n + " OR email ILIKE $" + n + " OR employee_id ILIKE $" + n + " OR upn ILIKE $" + n + ")"
	}
	const key = `lower(COALESCE(NULLIF(display_name,''), email))`
	if afterID != "" {
		args = append(args, afterKey, afterID)
		where += fmt.Sprintf(" AND (%s, id) > ($%d, $%d::uuid)", key, len(args)-1, len(args))
	}
	args = append(args, limit+1)
	rows, err := r.pool.Query(ctx, `SELECT oidc_subject, `+key+`, id::text FROM adrient.users WHERE `+where+
		fmt.Sprintf(` ORDER BY %s, id LIMIT $%d`, key, len(args)), args...)
	if err != nil {
		return SubjectPage{}, fmt.Errorf("users.ListOIDCSubjectsPage: %w", err)
	}
	defer rows.Close()
	var subs, keys, ids []string
	for rows.Next() {
		var s, k, id string
		if err := rows.Scan(&s, &k, &id); err != nil {
			return SubjectPage{}, err
		}
		subs, keys, ids = append(subs, s), append(keys, k), append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return SubjectPage{}, err
	}
	p := SubjectPage{Subjects: subs}
	if len(subs) > limit {
		p.Subjects = subs[:limit]
		p.NextKey, p.NextID = keys[limit-1], ids[limit-1]
	}
	return p, nil
}
