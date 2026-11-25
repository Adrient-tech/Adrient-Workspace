package iamops

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

// Alert is one risk alert in the triage queue.
type Alert struct {
	ID           string          `json:"id"`
	UserID       string          `json:"user_id"`
	Email        string          `json:"email"`
	IP           string          `json:"ip"`
	Rule         string          `json:"rule"`
	Severity     string          `json:"severity"`
	Status       string          `json:"status"`
	Title        string          `json:"title"`
	Evidence     json.RawMessage `json:"evidence"`
	Recommended  string          `json:"recommended"`
	AssigneeID   string          `json:"assignee_id"`
	AssigneeName string          `json:"assignee_name"`
	SLADueAt     *time.Time      `json:"sla_due_at"`
	SLABreached  bool            `json:"sla_breached"`
	ResolvedAt   *time.Time      `json:"resolved_at"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// AlertFilter narrows the queue.
type AlertFilter struct {
	Status, Severity, Assignee, Q, UserID, Cursor string
	Limit                                         int
}

const alertCols = `a.id::text, COALESCE(a.user_id::text,''), a.email, a.ip, a.rule, a.severity, a.status, a.title, a.evidence, a.recommended,
COALESCE(a.assignee_id::text,''), COALESCE(au.display_name, au.email, ''), a.sla_due_at, a.resolved_at, a.created_at, a.updated_at`

func scanAlert(row interface{ Scan(...any) error }, now time.Time) (Alert, error) {
	var x Alert
	if err := row.Scan(&x.ID, &x.UserID, &x.Email, &x.IP, &x.Rule, &x.Severity, &x.Status, &x.Title, &x.Evidence, &x.Recommended,
		&x.AssigneeID, &x.AssigneeName, &x.SLADueAt, &x.ResolvedAt, &x.CreatedAt, &x.UpdatedAt); err != nil {
		return x, err
	}
	open := x.Status == "open" || x.Status == "investigating"
	x.SLABreached = open && x.SLADueAt != nil && now.After(*x.SLADueAt)
	return x, nil
}

// ListAlerts returns a page of alerts, newest first. The cursor is "created_at|id" of the last row.
func (s *Service) ListAlerts(ctx context.Context, orgID string, f AlertFilter) ([]Alert, string, int, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	where := []string{"a.org_id::text=$1"}
	args := []any{orgID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}
	switch f.Status {
	case "", "all":
	case "active":
		where = append(where, "a.status IN ('open','investigating')")
	case "resolved":
		where = append(where, "a.status NOT IN ('open','investigating')")
	default:
		add("a.status = ?", f.Status)
	}
	if f.Severity != "" {
		add("a.severity = ?", f.Severity)
	}
	if f.Assignee == "me" || f.Assignee == "none" {
		if f.Assignee == "none" {
			where = append(where, "a.assignee_id IS NULL")
		}
	} else if f.Assignee != "" {
		add("a.assignee_id::text = ?", f.Assignee)
	}
	if f.UserID != "" {
		add("a.user_id::text = ?", f.UserID)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		add("(a.email ILIKE ? OR a.title ILIKE ? OR a.ip ILIKE ?)", "%"+q+"%")
	}
	var total int
	_ = s.Pool.QueryRow(ctx, "SELECT count(*) FROM adrient.iam_alerts a WHERE "+strings.Join(where, " AND "), args...).Scan(&total)
	if f.Cursor != "" {
		parts := strings.SplitN(f.Cursor, "|", 2)
		if len(parts) == 2 {
			args = append(args, parts[0], parts[1])
			where = append(where, fmt.Sprintf("(a.created_at, a.id::text) < ($%d::timestamptz, $%d)", len(args)-1, len(args)))
		}
	}
	args = append(args, f.Limit+1)
	rows, err := s.Pool.Query(ctx, `SELECT `+alertCols+` FROM adrient.iam_alerts a LEFT JOIN adrient.users au ON au.id=a.assignee_id WHERE `+strings.Join(where, " AND ")+
		fmt.Sprintf(` ORDER BY a.created_at DESC, a.id::text DESC LIMIT $%d`, len(args)), args...)
	if err != nil {
		return nil, "", 0, err
	}
	defer rows.Close()
	out := []Alert{}
	now := s.now()
	for rows.Next() {
		x, err := scanAlert(rows, now)
		if err != nil {
			return nil, "", 0, err
		}
		out = append(out, x)
	}
	next := ""
	if len(out) > f.Limit {
		out = out[:f.Limit]
		last := out[len(out)-1]
		next = last.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID
	}
	return out, next, total, rows.Err()
}

// Comment is one line of an alert's timeline.
type Comment struct {
	ID        int64     `json:"id"`
	AuthorID  string    `json:"author_id"`
	Author    string    `json:"author"`
	Kind      string    `json:"kind"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// AlertDetail is an alert with its comments and the sign-in events around it.
type AlertDetail struct {
	Alert    Alert     `json:"alert"`
	Comments []Comment `json:"comments"`
	Events   []Event   `json:"events"`
}

// GetAlert loads one alert with its timeline.
func (s *Service) GetAlert(ctx context.Context, orgID, id string) (AlertDetail, error) {
	var d AlertDetail
	row := s.Pool.QueryRow(ctx, `SELECT `+alertCols+` FROM adrient.iam_alerts a LEFT JOIN adrient.users au ON au.id=a.assignee_id WHERE a.org_id::text=$1 AND a.id::text=$2`, orgID, id)
	a, err := scanAlert(row, s.now())
	if err != nil {
		return d, ErrNotFound
	}
	d.Alert = a
	d.Comments = []Comment{}
	rows, err := s.Pool.Query(ctx, `SELECT c.id, COALESCE(c.author_id::text,''), COALESCE(u.display_name,u.email,'system'), c.kind, c.body, c.created_at
FROM adrient.iam_alert_comments c LEFT JOIN adrient.users u ON u.id=c.author_id WHERE c.alert_id::text=$1 ORDER BY c.id`, id)
	if err == nil {
		for rows.Next() {
			var c Comment
			if rows.Scan(&c.ID, &c.AuthorID, &c.Author, &c.Kind, &c.Body, &c.CreatedAt) == nil {
				d.Comments = append(d.Comments, c)
			}
		}
		rows.Close()
	}
	// Timeline: the account's (or the address's) sign-in events around the alert.
	var evs []Event
	var q string
	var arg any
	switch {
	case a.UserID != "":
		q, arg = `WHERE org_id::text=$1 AND user_id::text=$2`, a.UserID
	case a.IP != "":
		q, arg = `WHERE org_id::text=$1 AND ip=$2`, a.IP
	}
	if q != "" {
		evs, _ = s.queryEvents(ctx, `SELECT `+eventCols+` FROM adrient.login_events `+q+` AND created_at BETWEEN $3::timestamptz - interval '48 hours' AND $3::timestamptz + interval '48 hours' ORDER BY id DESC LIMIT 100`, orgID, arg, a.CreatedAt)
	}
	if evs == nil {
		evs = []Event{}
	}
	d.Events = evs
	return d, nil
}

const eventCols = `id, COALESCE(user_id::text,''), email, ip, user_agent, method, success, reason, country, city, device_fp, mfa_result, created_at`

func (s *Service) queryEvents(ctx context.Context, q string, args ...any) ([]Event, error) {
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.UserID, &e.Email, &e.IP, &e.UserAgent, &e.Method, &e.Success, &e.Reason, &e.Country, &e.City, &e.DeviceFP, &e.MFAResult, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EventFilter narrows the event list.
type EventFilter struct {
	UserID, IP, Q, Result string // Result: ok | failed | suspicious
	Cursor                string // last id
	Limit                 int
}

// ListEvents pages through sign-in events, newest first, by id.
func (s *Service) ListEvents(ctx context.Context, orgID string, f EventFilter) ([]Event, string, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	where := []string{"org_id::text=$1"}
	args := []any{orgID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(args))))
	}
	if f.UserID != "" {
		add("user_id::text = ?", f.UserID)
	}
	if f.IP != "" {
		add("ip = ?", f.IP)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		add("(email ILIKE ? OR ip ILIKE ?)", "%"+q+"%")
	}
	switch f.Result {
	case "ok":
		where = append(where, "success")
	case "failed":
		where = append(where, "NOT success")
	case "suspicious":
		where = append(where, "suspicious")
	}
	if f.Cursor != "" {
		add("id < ?::bigint", f.Cursor)
	}
	args = append(args, f.Limit+1)
	evs, err := s.queryEvents(ctx, `SELECT `+eventCols+` FROM adrient.login_events WHERE `+strings.Join(where, " AND ")+fmt.Sprintf(` ORDER BY id DESC LIMIT $%d`, len(args)), args...)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(evs) > f.Limit {
		evs = evs[:f.Limit]
		next = fmt.Sprint(evs[len(evs)-1].ID)
	}
	return evs, next, nil
}

// UpdateAlert changes the status and/or assignee of an alert, with a timeline entry.
func (s *Service) UpdateAlert(ctx context.Context, orgID string, a Actor, id, status, assignee string, assignSet bool, note string) error {
	if !Has(a.Roles, CapTriage) {
		return fmt.Errorf("%w: your role cannot triage alerts", ErrForbidden)
	}
	valid := map[string]bool{"open": true, "investigating": true, "false_positive": true, "compromised": true, "policy": true}
	if status != "" && !valid[status] {
		return bad("unknown status")
	}
	var cur string
	if err := scanOne(s.Pool.QueryRow(ctx, `SELECT status FROM adrient.iam_alerts WHERE org_id::text=$1 AND id::text=$2`, orgID, id), &cur); err != nil {
		return err
	}
	if status != "" && status != cur {
		resolved := status != "open" && status != "investigating"
		if _, err := s.Pool.Exec(ctx, `UPDATE adrient.iam_alerts SET status=$3, updated_at=now(), resolved_at=CASE WHEN $4 THEN now() ELSE NULL END, resolved_by=CASE WHEN $4 THEN $5::uuid ELSE NULL END
WHERE org_id::text=$1 AND id::text=$2`, orgID, id, status, resolved, a.ID); err != nil {
			return err
		}
		s.addComment(ctx, orgID, id, a.ID, "status", fmt.Sprintf("Status changed from %s to %s. %s", cur, status, note))
		s.audit(ctx, orgID, a.ID, "alert."+status, map[string]any{"alert": id})
	}
	if assignSet {
		if _, err := s.Pool.Exec(ctx, `UPDATE adrient.iam_alerts SET assignee_id=NULLIF($3,'')::uuid, updated_at=now() WHERE org_id::text=$1 AND id::text=$2`, orgID, id, assignee); err != nil {
			return err
		}
		who := "nobody"
		if assignee != "" {
			_ = s.Pool.QueryRow(ctx, `SELECT COALESCE(display_name,email) FROM adrient.users WHERE id::text=$1`, assignee).Scan(&who)
		}
		s.addComment(ctx, orgID, id, a.ID, "assign", "Assigned to "+who+".")
	}
	return nil
}

func (s *Service) addComment(ctx context.Context, orgID, alertID, author, kind, body string) {
	_, _ = s.Pool.Exec(ctx, `INSERT INTO adrient.iam_alert_comments (alert_id, org_id, author_id, kind, body) SELECT id, org_id, NULLIF($3,'')::uuid, $4, $5 FROM adrient.iam_alerts WHERE org_id::text=$1 AND id::text=$2`,
		orgID, alertID, author, kind, strings.TrimSpace(body))
}

// Comment adds a note to an alert.
func (s *Service) Comment(ctx context.Context, orgID string, a Actor, id, body string) error {
	if !Has(a.Roles, CapTriage) {
		return fmt.Errorf("%w: your role cannot comment on alerts", ErrForbidden)
	}
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 2000 {
		return bad("write a comment of up to 2000 characters")
	}
	var one string
	if err := scanOne(s.Pool.QueryRow(ctx, `SELECT id::text FROM adrient.iam_alerts WHERE org_id::text=$1 AND id::text=$2`, orgID, id), &one); err != nil {
		return err
	}
	s.addComment(ctx, orgID, id, a.ID, "comment", body)
	return nil
}

// BulkAlerts applies a status and/or assignee to many alerts and reports how many changed.
func (s *Service) BulkAlerts(ctx context.Context, orgID string, a Actor, ids []string, status, assignee string, assignSet bool) (int, error) {
	if len(ids) == 0 || len(ids) > 200 {
		return 0, bad("choose between 1 and 200 alerts")
	}
	n := 0
	for _, id := range ids {
		if err := s.UpdateAlert(ctx, orgID, a, id, status, assignee, assignSet, "(bulk)"); err == nil {
			n++
		} else if err == ErrForbidden {
			return n, err
		}
	}
	return n, nil
}

// AlertSummary counts for the dashboard header.
type AlertSummary struct {
	Open, Investigating, Critical, High, Breached, Today int
}

// Summary returns queue counters.
func (s *Service) Summary(ctx context.Context, orgID string) AlertSummary {
	var x AlertSummary
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status='open'), count(*) FILTER (WHERE status='investigating'),
count(*) FILTER (WHERE status IN ('open','investigating') AND severity='critical'), count(*) FILTER (WHERE status IN ('open','investigating') AND severity='high'),
count(*) FILTER (WHERE status IN ('open','investigating') AND sla_due_at < now()), count(*) FILTER (WHERE created_at > now() - interval '24 hours')
FROM adrient.iam_alerts WHERE org_id::text=$1`, orgID).Scan(&x.Open, &x.Investigating, &x.Critical, &x.High, &x.Breached, &x.Today)
	return x
}

// ---- allow and block lists ----------------------------------------------------------

// IPRule is one list entry.
type IPRule struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Scope     string     `json:"scope"`
	Value     string     `json:"value"`
	Reason    string     `json:"reason"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	Expired   bool       `json:"expired"`
}

// ListRules lists entries, newest first.
func (s *Service) ListRules(ctx context.Context, orgID string) ([]IPRule, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, kind, scope, value, reason, expires_at, created_at FROM adrient.iam_ip_rules WHERE org_id::text=$1 ORDER BY created_at DESC LIMIT 500`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IPRule{}
	for rows.Next() {
		var r IPRule
		if err := rows.Scan(&r.ID, &r.Kind, &r.Scope, &r.Value, &r.Reason, &r.ExpiresAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Expired = r.ExpiresAt != nil && r.ExpiresAt.Before(s.now())
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddRule adds an allow or block entry.
func (s *Service) AddRule(ctx context.Context, orgID string, a Actor, r IPRule, hours int) (IPRule, error) {
	if !Has(a.Roles, CapListEdit) {
		return r, fmt.Errorf("%w: your role cannot edit the lists", ErrForbidden)
	}
	if r.Kind != "allow" && r.Kind != "block" {
		return r, bad("choose allow or block")
	}
	r.Value = strings.TrimSpace(r.Value)
	switch r.Scope {
	case "ip", "cidr":
		v, err := ValidateNetworkEntry(r.Value)
		if err != nil {
			return r, bad("%v", err)
		}
		r.Value = v
		r.Scope = "ip"
		if strings.Contains(v, "/") {
			r.Scope = "cidr"
		}
	case "country":
		r.Value = strings.ToUpper(r.Value)
		if len(r.Value) != 2 {
			return r, bad("a country is a two-letter code such as IN or US")
		}
	default:
		return r, bad("choose an address, a range or a country")
	}
	if len(strings.TrimSpace(r.Reason)) < 3 {
		return r, bad("a reason is required")
	}
	var exp *time.Time
	if hours > 0 {
		e := s.now().Add(time.Duration(hours) * time.Hour)
		exp = &e
	}
	if r.Kind == "block" && r.Scope != "country" {
		// Never let an admin lock themselves out by blocking an address they are using.
		if myip, _ := ctx.Value(ipKey{}).(string); myip != "" && inList(myip, []string{r.Value}) {
			return r, bad("that entry includes the address you are using right now")
		}
	}
	err := s.Pool.QueryRow(ctx, `INSERT INTO adrient.iam_ip_rules (org_id, kind, scope, value, reason, expires_at, created_by) VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::uuid) RETURNING id::text, created_at`,
		orgID, r.Kind, r.Scope, r.Value, strings.TrimSpace(r.Reason), exp, a.ID).Scan(&r.ID, &r.CreatedAt)
	r.ExpiresAt = exp
	s.audit(ctx, orgID, a.ID, "list."+r.Kind, map[string]any{"scope": r.Scope, "value": r.Value, "reason": r.Reason, "hours": hours})
	return r, err
}

// DeleteRule removes an entry.
func (s *Service) DeleteRule(ctx context.Context, orgID string, a Actor, id string) error {
	if !Has(a.Roles, CapListEdit) {
		return fmt.Errorf("%w: your role cannot edit the lists", ErrForbidden)
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM adrient.iam_ip_rules WHERE org_id::text=$1 AND id::text=$2`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	s.audit(ctx, orgID, a.ID, "list.remove", map[string]any{"rule": id})
	return nil
}

// CheckSignIn is the sign-in gate: an address or country on the block list is
// refused unless an allow entry covers it. It fails open on errors.
func (s *Service) CheckSignIn(ctx context.Context, orgID, ip, country string) error {
	if s == nil || s.Pool == nil {
		return nil
	}
	rows, err := s.Pool.Query(ctx, `SELECT kind, scope, value, reason FROM adrient.iam_ip_rules WHERE (expires_at IS NULL OR expires_at > now()) AND ($1='' OR org_id::text=$1)`, orgID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	p := net.ParseIP(strings.TrimSpace(ip))
	var blockedBy string
	allowed := false
	for rows.Next() {
		var kind, scope, val, reason string
		if rows.Scan(&kind, &scope, &val, &reason) != nil {
			continue
		}
		match := false
		switch scope {
		case "country":
			match = country != "" && strings.EqualFold(country, val)
		default:
			match = p != nil && inList(ip, []string{val})
		}
		if !match {
			continue
		}
		if kind == "allow" {
			allowed = true
		} else {
			blockedBy = val
		}
	}
	if blockedBy != "" && !allowed {
		return fmt.Errorf("blocked_by_list: %s", blockedBy)
	}
	return nil
}

// Quarantined reports whether the account is currently read-only.
func (s *Service) Quarantined(ctx context.Context, userID string) bool {
	var one int
	err := s.Pool.QueryRow(ctx, `SELECT 1 FROM adrient.iam_restrictions WHERE user_id::text=$1 AND kind='quarantine' AND (expires_at IS NULL OR expires_at > now())`, userID).Scan(&one)
	return err == nil
}
