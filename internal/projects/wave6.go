package projects

import "context"

// ValidStatus reports whether s is one of the issue statuses.
func ValidStatus(s string) bool {
	switch s {
	case "backlog", "todo", "in_progress", "done", "canceled":
		return true
	}
	return false
}

// SetArchived archives or restores issues of the org. Only done or canceled
// issues may be archived, so open work never disappears from the board.
// It returns how many issues changed.
func (r *Repository) SetArchived(ctx context.Context, org string, ids []string, archived bool) (int, error) {
	if len(ids) == 0 {
		return 0, invalid("no issues given")
	}
	if len(ids) > 500 {
		return 0, invalid("at most 500 issues at a time")
	}
	for _, id := range ids {
		if !isUUID(id) {
			return 0, invalid("bad issue id")
		}
	}
	filter := ``
	if archived {
		filter = ` AND i.status IN ('done','canceled')`
	}
	tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.project_issue_extras (issue_id, org_id, archived)
		SELECT i.id, i.org_id, $3 FROM adrient.project_issues i
		WHERE i.id = ANY($1::uuid[]) AND i.org_id=$2 AND i.trashed_at IS NULL`+filter+`
		ON CONFLICT (issue_id) DO UPDATE SET archived=$3, updated_at=now()`, ids, org, archived)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// WipLimit is a soft cap on the number of issues in one status of a team. It is
// a warning only: nothing is ever blocked.
type WipLimit struct {
	Status string `json:"status"`
	Max    int    `json:"max"`
}

// ListWip returns the limits of a team.
func (r *Repository) ListWip(ctx context.Context, org, team string) ([]WipLimit, error) {
	if !isUUID(team) {
		return nil, invalid("bad team id")
	}
	rows, err := r.pool.Query(ctx, `SELECT status, max_issues FROM adrient.project_wip_limits WHERE org_id=$1 AND team_id=$2 ORDER BY status`, org, team)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WipLimit{}
	for rows.Next() {
		var w WipLimit
		if err := rows.Scan(&w.Status, &w.Max); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// SetWip sets (max 1 to 999) or clears (max 0) a limit.
func (r *Repository) SetWip(ctx context.Context, org, team, status string, max int) error {
	if !isUUID(team) || !ValidStatus(status) {
		return invalid("bad team or status")
	}
	if max < 0 || max > 999 {
		return invalid("the limit must be between 1 and 999, or 0 to remove it")
	}
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.project_teams WHERE id=$1 AND org_id=$2`, team, org).Scan(&n); err != nil || n == 0 {
		return ErrNotFound
	}
	if max == 0 {
		_, err := r.pool.Exec(ctx, `DELETE FROM adrient.project_wip_limits WHERE team_id=$1 AND status=$2 AND org_id=$3`, team, status, org)
		return err
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.project_wip_limits (org_id, team_id, status, max_issues) VALUES ($1,$2,$3,$4)
		ON CONFLICT (team_id, status) DO UPDATE SET max_issues=$4`, org, team, status, max)
	return err
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'):
		default:
			return false
		}
	}
	return true
}

// WipOver returns how many issues exceed the limit (0 when within it).
func WipOver(count, max int) int {
	if max <= 0 || count <= max {
		return 0
	}
	return count - max
}
