package crm

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// Team is a sales team.
type Team struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	ManagerID string   `json:"manager_id"`
	Members   []string `json:"members"`
}

func (r *Repository) ListTeams(ctx context.Context, org string) ([]Team, error) {
	rows, err := r.pool.Query(ctx, `SELECT t.id::text, t.name, coalesce(t.manager_id::text,''),
		coalesce((SELECT json_agg(m.user_id::text ORDER BY m.user_id) FROM adrient.crm_team_members m WHERE m.team_id=t.id),'[]')
		FROM adrient.crm_teams t WHERE t.org_id=$1 ORDER BY lower(t.name)`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Team{}
	for rows.Next() {
		var t Team
		var m []byte
		if err := rows.Scan(&t.ID, &t.Name, &t.ManagerID, &m); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(m, &t.Members)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) SaveTeam(ctx context.Context, org, id string, t Team) (Team, error) {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return t, invalid("team name is required")
	}
	for _, u := range append([]string{t.ManagerID}, t.Members...) {
		if u == "" {
			continue
		}
		var ok bool
		if !uuidRe.MatchString(u) || r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE id=$1 AND org_id=$2)`, u, org).Scan(&ok) != nil || !ok {
			return t, invalid("unknown user %q", u)
		}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return t, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if id == "" {
		err = tx.QueryRow(ctx, `INSERT INTO adrient.crm_teams (org_id,name,manager_id) VALUES ($1,$2,$3) RETURNING id::text`, org, t.Name, nz(t.ManagerID)).Scan(&t.ID)
	} else {
		t.ID = id
		var n int
		err = tx.QueryRow(ctx, `WITH u AS (UPDATE adrient.crm_teams SET name=$3, manager_id=$4 WHERE org_id=$1 AND id=$2 RETURNING 1) SELECT count(*) FROM u`, org, id, t.Name, nz(t.ManagerID)).Scan(&n)
		if err == nil && n == 0 {
			return t, ErrNotFound
		}
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM adrient.crm_team_members WHERE team_id=$1`, id)
		}
	}
	if err != nil {
		return t, err
	}
	for _, u := range t.Members {
		if _, err := tx.Exec(ctx, `INSERT INTO adrient.crm_team_members (team_id,user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, t.ID, u); err != nil {
			return t, err
		}
	}
	return t, tx.Commit(ctx)
}

func (r *Repository) DeleteTeam(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_teams WHERE org_id=$1 AND id=$2`, org, id)
}

// Territory assigns companies to a team by industry or name keyword.
type Territory struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	TeamID     string   `json:"team_id"`
	Industries []string `json:"industries"`
	Keywords   []string `json:"keywords"`
}

func (r *Repository) ListTerritories(ctx context.Context, org string) ([]Territory, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, coalesce(team_id::text,''), match FROM adrient.crm_territories WHERE org_id=$1 ORDER BY lower(name)`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Territory{}
	for rows.Next() {
		var t Territory
		var m []byte
		if err := rows.Scan(&t.ID, &t.Name, &t.TeamID, &m); err != nil {
			return nil, err
		}
		var mm struct{ Industries, Keywords []string }
		_ = json.Unmarshal(m, &mm)
		t.Industries, t.Keywords = mm.Industries, mm.Keywords
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *Repository) SaveTerritory(ctx context.Context, org, id string, t Territory) (Territory, error) {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return t, invalid("territory name is required")
	}
	m, _ := json.Marshal(map[string]any{"industries": nonNilStrs(t.Industries), "keywords": nonNilStrs(t.Keywords)})
	if id == "" {
		err := r.pool.QueryRow(ctx, `INSERT INTO adrient.crm_territories (org_id,name,team_id,match) VALUES ($1,$2,$3,$4) RETURNING id::text`, org, t.Name, nz(t.TeamID), m).Scan(&t.ID)
		return t, err
	}
	if err := r.del(ctx, `UPDATE adrient.crm_territories SET name=$3,team_id=$4,match=$5 WHERE org_id=$1 AND id=$2`, org, id, t.Name, nz(t.TeamID), m); err != nil {
		return t, err
	}
	t.ID = id
	return t, nil
}

func (r *Repository) DeleteTerritory(ctx context.Context, org, id string) error {
	return r.del(ctx, `DELETE FROM adrient.crm_territories WHERE org_id=$1 AND id=$2`, org, id)
}

func nonNilStrs(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// MatchTerritory returns the first territory whose industry or keyword matches a company.
func MatchTerritory(ts []Territory, industry, name string) string {
	for _, t := range ts {
		for _, i := range t.Industries {
			if industry != "" && strings.EqualFold(strings.TrimSpace(i), industry) {
				return t.ID
			}
		}
		for _, k := range t.Keywords {
			if k = strings.ToLower(strings.TrimSpace(k)); k != "" && strings.Contains(strings.ToLower(name), k) {
				return t.ID
			}
		}
	}
	return ""
}

// ApplyTerritories sets territory_id on companies that match a territory and
// returns how many changed. Companies already in a territory are left alone.
func (r *Repository) ApplyTerritories(ctx context.Context, org string) (int, error) {
	ts, err := r.ListTerritories(ctx, org)
	if err != nil {
		return 0, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, industry, name FROM adrient.crm_companies WHERE org_id=$1 AND territory_id IS NULL`, org)
	if err != nil {
		return 0, err
	}
	type c struct{ id, ind, name string }
	var cs []c
	for rows.Next() {
		var x c
		_ = rows.Scan(&x.id, &x.ind, &x.name)
		cs = append(cs, x)
	}
	rows.Close()
	n := 0
	for _, x := range cs {
		if tid := MatchTerritory(ts, x.ind, x.name); tid != "" {
			if _, err := r.pool.Exec(ctx, `UPDATE adrient.crm_companies SET territory_id=$3 WHERE org_id=$1 AND id=$2`, org, x.id, tid); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

// UserScope is a user's visibility and mail-logging preference.
type UserScope struct {
	UserID      string `json:"user_id"`
	Scope       string `json:"scope"`
	AutoLogMail bool   `json:"auto_log_mail"`
	Effective   string `json:"effective"`
}

// ResolveScope returns the scope a non-admin user sees: their own row, else the org default.
func (r *Repository) ResolveScope(ctx context.Context, org, user string) (string, error) {
	var s string
	err := r.pool.QueryRow(ctx, `SELECT coalesce((SELECT scope FROM adrient.crm_user_scope WHERE org_id=$1 AND user_id=$2),
		(SELECT default_scope FROM adrient.crm_settings WHERE org_id=$1), 'all')`, org, user).Scan(&s)
	return s, err
}

func (r *Repository) ListUserScopes(ctx context.Context, org string) ([]UserScope, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id::text, scope, auto_log_mail FROM adrient.crm_user_scope WHERE org_id=$1`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserScope{}
	for rows.Next() {
		var u UserScope
		if err := rows.Scan(&u.UserID, &u.Scope, &u.AutoLogMail); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *Repository) SetUserScope(ctx context.Context, org, user, scope string) error {
	if scope != ScopeOwn && scope != ScopeTeam && scope != ScopeAll {
		return invalid("scope must be own, team or all")
	}
	var ok bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE id=$1 AND org_id=$2)`, user, org).Scan(&ok); err != nil || !ok {
		return invalid("unknown user")
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.crm_user_scope (org_id,user_id,scope) VALUES ($1,$2,$3)
		ON CONFLICT (org_id,user_id) DO UPDATE SET scope=EXCLUDED.scope`, org, user, scope)
	return err
}

func (r *Repository) SetAutoLogMail(ctx context.Context, org, user string, on bool) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.crm_user_scope (org_id,user_id,auto_log_mail,scope)
		VALUES ($1,$2,$3,coalesce((SELECT default_scope FROM adrient.crm_settings WHERE org_id=$1),'all'))
		ON CONFLICT (org_id,user_id) DO UPDATE SET auto_log_mail=EXCLUDED.auto_log_mail`, org, user, on)
	return err
}

func (r *Repository) AutoLogMailEnabled(ctx context.Context, org, user string) bool {
	var on bool
	_ = r.pool.QueryRow(ctx, `SELECT auto_log_mail FROM adrient.crm_user_scope WHERE org_id=$1 AND user_id=$2`, org, user).Scan(&on)
	return on
}

// Quota is a monthly revenue target for a user.
type Quota struct {
	UserID string  `json:"user_id"`
	Period string  `json:"period"` // YYYY-MM
	Target float64 `json:"target"`
}

func (r *Repository) SetQuota(ctx context.Context, org string, q Quota) error {
	t, err := time.Parse("2006-01", q.Period)
	if err != nil {
		return invalid("period must be YYYY-MM")
	}
	if q.Target < 0 {
		return invalid("target cannot be negative")
	}
	var ok bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM adrient.users WHERE id=$1 AND org_id=$2)`, q.UserID, org).Scan(&ok); err != nil || !ok {
		return invalid("unknown user")
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.crm_quotas (org_id,user_id,period,target) VALUES ($1,$2,$3,$4)
		ON CONFLICT (org_id,user_id,period) DO UPDATE SET target=EXCLUDED.target`, org, q.UserID, t, q.Target)
	return err
}

// LeaderRow is one rep on the leaderboard.
type LeaderRow struct {
	UserID   string  `json:"user_id"`
	Name     string  `json:"name"`
	Won      float64 `json:"won"`
	WonCount int     `json:"won_count"`
	Open     float64 `json:"open"`
	Weighted float64 `json:"weighted"`
	Target   float64 `json:"target"`
	Pct      float64 `json:"pct"`
	Activity int     `json:"activities"`
}

// Leaderboard ranks owners by won value in a month (YYYY-MM) against their quota.
// It only counts deals the viewer may see.
func (r *Repository) Leaderboard(ctx context.Context, org, period string) ([]LeaderRow, error) {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return nil, invalid("period must be YYYY-MM")
	}
	set, err := r.GetSettings(ctx, org)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT u.id::text, u.display_name,
		coalesce(sum(d.value) FILTER (WHERE d.stage='won' AND d.closed_at >= $2 AND d.closed_at < $3),0)::float8,
		count(*) FILTER (WHERE d.stage='won' AND d.closed_at >= $2 AND d.closed_at < $3),
		coalesce(sum(d.value) FILTER (WHERE d.stage NOT IN ('won','lost')),0)::float8,
		coalesce((SELECT target FROM adrient.crm_quotas q WHERE q.org_id=$1 AND q.user_id=u.id AND q.period=$2),0)::float8,
		(SELECT count(*) FROM adrient.crm_activities a WHERE a.org_id=$1 AND a.user_id=u.id AND a.occurred_at >= $2 AND a.occurred_at < $3)
		FROM adrient.users u LEFT JOIN adrient.crm_deals d ON d.owner_id=u.id AND d.org_id=$1 AND `+ViewerFrom(ctx).ownerSQL("d")+`
		WHERE u.org_id=$1 GROUP BY u.id, u.display_name
		HAVING count(d.id) > 0 OR EXISTS (SELECT 1 FROM adrient.crm_quotas q WHERE q.org_id=$1 AND q.user_id=u.id AND q.period=$2)
		ORDER BY 3 DESC, 2`, org, t, t.AddDate(0, 1, 0))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LeaderRow{}
	for rows.Next() {
		var l LeaderRow
		if err := rows.Scan(&l.UserID, &l.Name, &l.Won, &l.WonCount, &l.Open, &l.Target, &l.Activity); err != nil {
			return nil, err
		}
		if l.Target > 0 {
			l.Pct = round2(l.Won / l.Target * 100)
		}
		out = append(out, l)
	}
	_ = set
	return out, rows.Err()
}
