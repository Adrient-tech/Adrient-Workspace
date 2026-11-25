package tickets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// SLARule is an SLA policy selected by priority, ticket type and customer.
type SLARule struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Position         int    `json:"position"`
	Enabled          bool   `json:"enabled"`
	MatchPriority    string `json:"match_priority"`
	MatchType        string `json:"match_type"`
	MatchCustomer    string `json:"match_customer"`
	FirstResponseMin int    `json:"first_response_min"`
	ResolveMin       int    `json:"resolve_min"`
	BusinessHours    bool   `json:"business_hours"`
	CalendarID       string `json:"calendar_id"`
}

// Matches reports whether a rule applies to a ticket. Empty matchers match anything.
func (s SLARule) Matches(priority, ttype, email string) bool {
	if !s.Enabled {
		return false
	}
	if s.MatchPriority != "" && s.MatchPriority != priority {
		return false
	}
	if s.MatchType != "" && s.MatchType != ttype {
		return false
	}
	if c := strings.ToLower(strings.TrimSpace(s.MatchCustomer)); c != "" {
		e := strings.ToLower(email)
		if !(e == c || strings.HasSuffix(e, "@"+strings.TrimPrefix(c, "@"))) {
			return false
		}
	}
	return true
}

// Validate checks a rule.
func (s *SLARule) Validate() error {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" || len(s.Name) > 80 {
		return errors.New("name is required (max 80 characters)")
	}
	if s.MatchPriority != "" && !validPriority(s.MatchPriority) {
		return errors.New("unknown priority")
	}
	if s.MatchType != "" && NormalizeType(s.MatchType) != s.MatchType {
		return errors.New("unknown ticket type")
	}
	if s.FirstResponseMin < 0 || s.ResolveMin < 0 || s.FirstResponseMin > 525600 || s.ResolveMin > 525600 {
		return errors.New("targets must be between 0 and 525600 minutes")
	}
	if s.FirstResponseMin == 0 && s.ResolveMin == 0 {
		return errors.New("set a first-response or a resolution target")
	}
	if s.ResolveMin > 0 && s.FirstResponseMin > s.ResolveMin {
		return errors.New("first response cannot be later than resolution")
	}
	return nil
}

// PickSLA returns the first matching rule, or nil.
func PickSLA(rules []SLARule, priority, ttype, email string) *SLARule {
	for i := range rules {
		if rules[i].Matches(priority, ttype, email) {
			return &rules[i]
		}
	}
	return nil
}

// ListSLARules returns a project's SLA rules in order.
func (r *Repository) ListSLARules(ctx context.Context, org, projectID string) ([]SLARule, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, position, enabled, match_priority, match_type, match_customer,
		first_response_min, resolve_min, business_hours, COALESCE(calendar_id::text,'')
		FROM adrient.ticket_sla_rules WHERE org_id=$1 AND project_id=$2 ORDER BY position, created_at`, org, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SLARule{}
	for rows.Next() {
		var s SLARule
		if err := rows.Scan(&s.ID, &s.Name, &s.Position, &s.Enabled, &s.MatchPriority, &s.MatchType, &s.MatchCustomer,
			&s.FirstResponseMin, &s.ResolveMin, &s.BusinessHours, &s.CalendarID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveSLARule creates or updates a rule.
func (r *Repository) SaveSLARule(ctx context.Context, org, projectID string, s SLARule) (SLARule, error) {
	if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return s, err
	}
	if err := s.Validate(); err != nil {
		return s, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	var err error
	if s.ID == "" {
		var n int
		_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.ticket_sla_rules WHERE project_id=$1`, projectID).Scan(&n)
		if n >= 50 {
			return s, fmt.Errorf("%w: at most 50 SLA rules per project", ErrInvalid)
		}
		s.Position = n
		err = r.pool.QueryRow(ctx, `INSERT INTO adrient.ticket_sla_rules (org_id,project_id,name,position,enabled,match_priority,match_type,match_customer,first_response_min,resolve_min,business_hours,calendar_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id::text`,
			org, projectID, s.Name, s.Position, s.Enabled, s.MatchPriority, s.MatchType, strings.TrimSpace(s.MatchCustomer), s.FirstResponseMin, s.ResolveMin, s.BusinessHours, nullUUID(s.CalendarID)).Scan(&s.ID)
	} else {
		var id string
		err = r.pool.QueryRow(ctx, `UPDATE adrient.ticket_sla_rules SET name=$4,position=$5,enabled=$6,match_priority=$7,match_type=$8,match_customer=$9,first_response_min=$10,resolve_min=$11,business_hours=$12,calendar_id=$13
			WHERE id=$1 AND org_id=$2 AND project_id=$3 RETURNING id::text`,
			s.ID, org, projectID, s.Name, s.Position, s.Enabled, s.MatchPriority, s.MatchType, strings.TrimSpace(s.MatchCustomer), s.FirstResponseMin, s.ResolveMin, s.BusinessHours, nullUUID(s.CalendarID)).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return s, ErrNotFound
		}
	}
	return s, err
}

// DeleteSLARule removes a rule.
func (r *Repository) DeleteSLARule(ctx context.Context, org, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_sla_rules WHERE org_id=$1 AND id=$2`, org, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// slaInputs is what the SLA of one ticket depends on.
type slaInputs struct {
	Priority, Type, Email, GroupID string
	Created                        time.Time
	Resolved, FirstResponse        *time.Time
	PausedAt                       *time.Time
	PausedMin                      int
}

// slaResolver evaluates SLAs for many tickets of one project, caching calendars.
type slaResolver struct {
	r      *Repository
	org    string
	rules  []SLARule
	legacy map[string]*SLAPolicy
	cals   map[string]*Calendar
}

func (r *Repository) newSLAResolver(ctx context.Context, org, projectID string) (*slaResolver, error) {
	rules, err := r.ListSLARules(ctx, org, projectID)
	if err != nil {
		return nil, err
	}
	pols, err := r.SLAPolicies(ctx, org, projectID)
	if err != nil {
		return nil, err
	}
	legacy := map[string]*SLAPolicy{}
	for i := range pols {
		legacy[pols[i].Priority] = &pols[i]
	}
	return &slaResolver{r: r, org: org, rules: rules, legacy: legacy, cals: map[string]*Calendar{}}, nil
}

func (s *slaResolver) empty() bool { return len(s.rules) == 0 && len(s.legacy) == 0 }

func (s *slaResolver) calendar(ctx context.Context, ruleCal, groupID string) *Calendar {
	key := ruleCal + "|" + groupID
	if c, ok := s.cals[key]; ok {
		return c
	}
	var c *Calendar
	if ruleCal != "" {
		c = s.r.LoadCalendar(ctx, s.org, ruleCal)
	} else {
		var gc string
		if groupID != "" {
			_ = s.r.pool.QueryRow(ctx, `SELECT COALESCE(calendar_id::text,'') FROM adrient.ticket_groups WHERE id=$1 AND org_id=$2`, groupID, s.org).Scan(&gc)
		}
		c = s.r.LoadCalendar(ctx, s.org, gc)
	}
	s.cals[key] = c
	return c
}

// eval computes the SLA state of one ticket.
func (s *slaResolver) eval(ctx context.Context, in slaInputs, now time.Time) SLAInfo {
	var fr, res int
	var cal *Calendar
	if rule := PickSLA(s.rules, in.Priority, in.Type, in.Email); rule != nil {
		fr, res = rule.FirstResponseMin, rule.ResolveMin
		if rule.BusinessHours {
			cal = s.calendar(ctx, rule.CalendarID, in.GroupID)
		}
	} else if p := s.legacy[in.Priority]; p != nil {
		fr, res = p.FirstResponseMin, p.ResolveMin
	} else {
		return SLAInfo{}
	}
	first := in.FirstResponse
	if in.Resolved != nil && first == nil {
		first = in.Resolved // closing a ticket counts as a response
	}
	mk := func(target int, done *time.Time, pausedAt *time.Time, paused int) SLAClock {
		c, _, left := SLAState(SLATimer{Created: in.Created, TargetMin: target, Calendar: cal, PausedMin: paused, PausedAt: pausedAt, DoneAt: done}, now)
		if target > 0 {
			l := left
			c.LeftMin = &l
			c.Paused = pausedAt != nil && done == nil
		}
		return c
	}
	return SLAInfo{FirstResponse: mk(fr, first, nil, 0), Resolution: mk(res, in.Resolved, in.PausedAt, in.PausedMin)}
}

// SLAStates computes SLA info for the recent tickets of a project.
func (r *Repository) SLAStates(ctx context.Context, org, projectID string, now time.Time) (map[string]SLAInfo, error) {
	rs, err := r.newSLAResolver(ctx, org, projectID)
	if err != nil {
		return nil, err
	}
	out := map[string]SLAInfo{}
	if rs.empty() {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT t.id::text, t.priority, t.ticket_type, t.requester_email, COALESCE(t.group_id::text,''), t.created_at, t.resolved_at,
		COALESCE(t.first_response_at, (SELECT min(c.created_at) FROM adrient.ticket_comments c WHERE c.ticket_id=t.id AND NOT c.is_internal
		   AND c.author_user_id IS NOT NULL AND c.author_user_id IS DISTINCT FROM t.requester_user_id)), t.sla_paused_at, t.sla_paused_min
		FROM adrient.tickets t WHERE t.project_id=$1 AND t.org_id=$2 AND t.merged_into IS NULL
		  AND (t.resolved_at IS NULL OR t.resolved_at > now() - interval '30 days')
		ORDER BY t.updated_at DESC LIMIT 5000`, projectID, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type row struct {
		id string
		in slaInputs
	}
	var list []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.id, &x.in.Priority, &x.in.Type, &x.in.Email, &x.in.GroupID, &x.in.Created, &x.in.Resolved, &x.in.FirstResponse, &x.in.PausedAt, &x.in.PausedMin); err != nil {
			return nil, err
		}
		list = append(list, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, x := range list {
		out[x.id] = rs.eval(ctx, x.in, now)
	}
	return out, nil
}

// SLAFor evaluates one ticket.
func (r *Repository) SLAFor(ctx context.Context, org, ticketID string, now time.Time) (SLAInfo, error) {
	var projectID string
	var in slaInputs
	err := r.pool.QueryRow(ctx, `SELECT t.project_id::text, t.priority, t.ticket_type, t.requester_email, COALESCE(t.group_id::text,''), t.created_at, t.resolved_at,
		COALESCE(t.first_response_at, (SELECT min(c.created_at) FROM adrient.ticket_comments c WHERE c.ticket_id=t.id AND NOT c.is_internal
		   AND c.author_user_id IS NOT NULL AND c.author_user_id IS DISTINCT FROM t.requester_user_id)), t.sla_paused_at, t.sla_paused_min
		FROM adrient.tickets t WHERE t.id=$1 AND t.org_id=$2`, ticketID, org).
		Scan(&projectID, &in.Priority, &in.Type, &in.Email, &in.GroupID, &in.Created, &in.Resolved, &in.FirstResponse, &in.PausedAt, &in.PausedMin)
	if errors.Is(err, pgx.ErrNoRows) {
		return SLAInfo{}, ErrNotFound
	}
	if err != nil {
		return SLAInfo{}, err
	}
	rs, err := r.newSLAResolver(ctx, org, projectID)
	if err != nil {
		return SLAInfo{}, err
	}
	return rs.eval(ctx, in, now), nil
}
