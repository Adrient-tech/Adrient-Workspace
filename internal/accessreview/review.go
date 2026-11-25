// Package accessreview runs periodic access review campaigns: reviewers keep or
// remove group members, external guests and admin roles, and the decisions are
// applied automatically when the campaign closes.
package accessreview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Kinds of campaign.
const (
	KindGroups = "group_membership"
	KindGuests = "external_guests"
	KindAdmins = "admin_roles"
)

var (
	ErrBad      = errors.New("invalid request")
	ErrNotFound = errors.New("not found")
	ErrForbid   = errors.New("not allowed")
)

// Service holds the dependencies.
type Service struct {
	Pool *pgxpool.Pool
	// Suspend blocks a guest whose access was removed.
	Suspend func(ctx context.Context, orgID, userID, actor, reason string) error
	// Notify tells a reviewer about pending work (best effort).
	Notify func(ctx context.Context, orgID, userID, title, body, url string)
	// Alert e-mails admins (best effort).
	Alert func(ctx context.Context, orgID, event, subject, body string)
	Audit func(ctx context.Context, orgID, actorID, action string, detail map[string]any)
	Now   func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) audit(ctx context.Context, org, actor, action string, d map[string]any) {
	if s.Audit != nil {
		s.Audit(ctx, org, actor, action, d)
	}
}

// Campaign is a review campaign with its progress.
type Campaign struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Kind           string          `json:"kind"`
	Scope          json.RawMessage `json:"scope"`
	DueAt          time.Time       `json:"due_at"`
	OnNoResponse   string          `json:"on_no_response"`
	RecurrenceDays *int            `json:"recurrence_days"`
	Status         string          `json:"status"`
	CreatedAt      time.Time       `json:"created_at"`
	ClosedAt       *time.Time      `json:"closed_at"`
	Total          int             `json:"total"`
	Pending        int             `json:"pending"`
	Kept           int             `json:"kept"`
	Removed        int             `json:"removed"`
}

// Item is one access to review.
type Item struct {
	ID           string     `json:"id"`
	CampaignID   string     `json:"campaign_id"`
	Campaign     string     `json:"campaign,omitempty"`
	SubjectID    string     `json:"subject_id"`
	SubjectEmail string     `json:"subject_email"`
	SubjectName  string     `json:"subject_name"`
	ResourceType string     `json:"resource_type"`
	ResourceID   string     `json:"resource_id"`
	ResourceName string     `json:"resource_name"`
	ReviewerID   string     `json:"reviewer_id"`
	ReviewerName string     `json:"reviewer_name"`
	Decision     string     `json:"decision"`
	Comment      string     `json:"comment"`
	DecidedAt    *time.Time `json:"decided_at"`
	AppliedAt    *time.Time `json:"applied_at"`
	ApplyResult  string     `json:"apply_result"`
	DueAt        *time.Time `json:"due_at,omitempty"`
}

// CreateInput describes a campaign.
type CreateInput struct {
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	GroupIDs       []string `json:"group_ids"`
	DueDays        int      `json:"due_days"`
	OnNoResponse   string   `json:"on_no_response"`
	RecurrenceDays *int     `json:"recurrence_days"`
}

// Create builds a campaign and its items. A reviewer never reviews their own access.
func (s *Service) Create(ctx context.Context, orgID, actor string, in CreateInput) (Campaign, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 120 {
		return Campaign{}, fmt.Errorf("%w: a campaign needs a name", ErrBad)
	}
	if in.Kind != KindGroups && in.Kind != KindGuests && in.Kind != KindAdmins {
		return Campaign{}, fmt.Errorf("%w: unknown kind", ErrBad)
	}
	if in.DueDays <= 0 {
		in.DueDays = 14
	}
	if in.DueDays > 365 {
		return Campaign{}, fmt.Errorf("%w: due within 365 days", ErrBad)
	}
	if in.OnNoResponse == "" {
		in.OnNoResponse = "keep"
	}
	if in.OnNoResponse != "keep" && in.OnNoResponse != "remove" {
		return Campaign{}, fmt.Errorf("%w: on_no_response is keep or remove", ErrBad)
	}
	if in.RecurrenceDays != nil && *in.RecurrenceDays < 7 {
		return Campaign{}, fmt.Errorf("%w: repeat at most weekly", ErrBad)
	}
	scope, _ := json.Marshal(map[string]any{"group_ids": in.GroupIDs})
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Campaign{}, err
	}
	defer tx.Rollback(ctx)
	due := s.now().AddDate(0, 0, in.DueDays)
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO adrient.access_review_campaigns (org_id, name, kind, scope, due_at, on_no_response, recurrence_days, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid) RETURNING id::text`, orgID, in.Name, in.Kind, scope, due, in.OnNoResponse, in.RecurrenceDays, actor).Scan(&id); err != nil {
		return Campaign{}, err
	}
	var n int
	switch in.Kind {
	case KindGroups:
		n, err = s.groupItems(ctx, tx, orgID, id, actor, in.GroupIDs)
	case KindGuests:
		n, err = s.guestItems(ctx, tx, orgID, id, actor)
	case KindAdmins:
		n, err = s.adminItems(ctx, tx, orgID, id, actor)
	}
	if err != nil {
		return Campaign{}, err
	}
	if n == 0 {
		return Campaign{}, fmt.Errorf("%w: there is nothing to review for this campaign", ErrBad)
	}
	if err := tx.Commit(ctx); err != nil {
		return Campaign{}, err
	}
	s.audit(ctx, orgID, actor, "review.create", map[string]any{"campaign": in.Name, "kind": in.Kind, "items": n})
	s.notifyReviewers(ctx, orgID, id, in.Name)
	return s.Get(ctx, orgID, id)
}

func (s *Service) groupItems(ctx context.Context, tx pgx.Tx, orgID, camp, actor string, groupIDs []string) (int, error) {
	q := `INSERT INTO adrient.access_review_items (campaign_id, org_id, subject_id, resource_type, resource_id, resource_name, reviewer_id)
SELECT $1::uuid, g.org_id, gm.user_id, 'group', g.id::text, g.name,
  CASE WHEN g.owner_id = gm.user_id THEN NULLIF($3,'')::uuid ELSE g.owner_id END
FROM adrient.groups g JOIN adrient.group_members gm ON gm.group_id=g.id
JOIN adrient.users u ON u.id=gm.user_id AND u.deleted_at IS NULL AND u.status='active'
WHERE g.org_id::text=$2`
	args := []any{camp, orgID, actor}
	if len(groupIDs) > 0 {
		q += ` AND g.id::text = ANY($4)`
		args = append(args, groupIDs)
	}
	tag, err := tx.Exec(ctx, q, args...)
	return int(tag.RowsAffected()), err
}

func (s *Service) guestItems(ctx context.Context, tx pgx.Tx, orgID, camp, actor string) (int, error) {
	tag, err := tx.Exec(ctx, `INSERT INTO adrient.access_review_items (campaign_id, org_id, subject_id, resource_type, resource_id, resource_name, reviewer_id)
SELECT $1::uuid, u.org_id, u.id, 'guest', u.id::text, u.user_type, COALESCE(NULLIF(u.sponsor_id, u.id), NULLIF($3,'')::uuid)
FROM adrient.users u WHERE u.org_id::text=$2 AND u.user_type IN ('guest','external') AND u.status='active' AND u.deleted_at IS NULL`, camp, orgID, actor)
	return int(tag.RowsAffected()), err
}

func (s *Service) adminItems(ctx context.Context, tx pgx.Tx, orgID, camp, actor string) (int, error) {
	// Reviewer: the campaign creator, or any other admin for the creator's own roles.
	tag, err := tx.Exec(ctx, `INSERT INTO adrient.access_review_items (campaign_id, org_id, subject_id, resource_type, resource_id, resource_name, reviewer_id)
SELECT $1::uuid, x.org_id, x.user_id, 'admin_role', x.role_key, x.role_name,
  CASE WHEN x.user_id::text = $3 THEN (SELECT oa.user_id FROM adrient.org_admins oa WHERE oa.org_id=x.org_id AND oa.user_id::text <> $3 ORDER BY oa.granted_at LIMIT 1)
       ELSE NULLIF($3,'')::uuid END
FROM (
  SELECT ra.org_id, ra.subject_id AS user_id, r.key AS role_key, r.name AS role_name
  FROM adrient.role_assignments ra JOIN adrient.roles r ON r.id=ra.role_id
  WHERE ra.org_id::text=$2 AND ra.subject_type='user' AND r.key IN ('owner','admin','user_admin','billing_admin','help_desk','readonly_admin')
  UNION
  SELECT oa.org_id, oa.user_id, 'admin', 'Admin' FROM adrient.org_admins oa WHERE oa.org_id::text=$2
) x JOIN adrient.users u ON u.id=x.user_id AND u.deleted_at IS NULL AND u.status='active'`, camp, orgID, actor)
	return int(tag.RowsAffected()), err
}

func (s *Service) notifyReviewers(ctx context.Context, orgID, camp, name string) {
	if s.Notify == nil {
		return
	}
	rows, err := s.Pool.Query(ctx, `SELECT reviewer_id::text, count(*) FROM adrient.access_review_items WHERE campaign_id::text=$1 AND reviewer_id IS NOT NULL GROUP BY 1`, camp)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var rid string
		var n int
		if rows.Scan(&rid, &n) == nil {
			s.Notify(ctx, orgID, rid, "Access review: "+name, fmt.Sprintf("%d access decisions are waiting for you.", n), "/access-reviews")
		}
	}
}

const campCols = `c.id::text, c.name, c.kind, c.scope, c.due_at, c.on_no_response, c.recurrence_days, c.status, c.created_at, c.closed_at,
(SELECT count(*) FROM adrient.access_review_items i WHERE i.campaign_id=c.id),
(SELECT count(*) FROM adrient.access_review_items i WHERE i.campaign_id=c.id AND i.decision='pending'),
(SELECT count(*) FROM adrient.access_review_items i WHERE i.campaign_id=c.id AND i.decision='keep'),
(SELECT count(*) FROM adrient.access_review_items i WHERE i.campaign_id=c.id AND i.decision='remove')`

func scanCamp(row pgx.Row) (Campaign, error) {
	var c Campaign
	err := row.Scan(&c.ID, &c.Name, &c.Kind, &c.Scope, &c.DueAt, &c.OnNoResponse, &c.RecurrenceDays, &c.Status, &c.CreatedAt, &c.ClosedAt, &c.Total, &c.Pending, &c.Kept, &c.Removed)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// Get returns one campaign of the org.
func (s *Service) Get(ctx context.Context, orgID, id string) (Campaign, error) {
	return scanCamp(s.Pool.QueryRow(ctx, `SELECT `+campCols+` FROM adrient.access_review_campaigns c WHERE c.org_id::text=$1 AND c.id::text=$2`, orgID, id))
}

// List returns the org's campaigns, newest first.
func (s *Service) List(ctx context.Context, orgID string) ([]Campaign, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+campCols+` FROM adrient.access_review_campaigns c WHERE c.org_id::text=$1 ORDER BY c.created_at DESC LIMIT 100`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Campaign{}
	for rows.Next() {
		c, err := scanCamp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

const itemCols = `i.id::text, i.campaign_id::text, c.name, i.subject_id::text, su.email, su.display_name, i.resource_type, i.resource_id, i.resource_name,
COALESCE(i.reviewer_id::text,''), COALESCE(ru.display_name, ru.email, ''), i.decision, i.comment, i.decided_at, i.applied_at, i.apply_result, c.due_at`

func scanItem(row pgx.Row) (Item, error) {
	var it Item
	err := row.Scan(&it.ID, &it.CampaignID, &it.Campaign, &it.SubjectID, &it.SubjectEmail, &it.SubjectName, &it.ResourceType, &it.ResourceID, &it.ResourceName,
		&it.ReviewerID, &it.ReviewerName, &it.Decision, &it.Comment, &it.DecidedAt, &it.AppliedAt, &it.ApplyResult, &it.DueAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

const itemFrom = ` FROM adrient.access_review_items i JOIN adrient.access_review_campaigns c ON c.id=i.campaign_id
JOIN adrient.users su ON su.id=i.subject_id LEFT JOIN adrient.users ru ON ru.id=i.reviewer_id`

// Items lists a campaign's items (all of them, for admins).
func (s *Service) Items(ctx context.Context, orgID, campaignID string) ([]Item, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+itemCols+itemFrom+` WHERE i.org_id::text=$1 AND i.campaign_id::text=$2 ORDER BY su.email, i.resource_name`, orgID, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Mine lists the open items waiting for reviewerID.
func (s *Service) Mine(ctx context.Context, orgID, reviewerID string) ([]Item, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+itemCols+itemFrom+` WHERE i.org_id::text=$1 AND i.reviewer_id::text=$2 AND c.status='open' AND i.decision='pending' ORDER BY c.due_at, su.email`, orgID, reviewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Decide records a reviewer's decision. Only the assigned reviewer (or an admin
// acting for them) may decide, and never for their own access.
func (s *Service) Decide(ctx context.Context, orgID, actor string, isAdmin bool, itemID, decision, comment string) error {
	if decision != "keep" && decision != "remove" {
		return fmt.Errorf("%w: decision is keep or remove", ErrBad)
	}
	var reviewer, subject, status string
	err := s.Pool.QueryRow(ctx, `SELECT COALESCE(i.reviewer_id::text,''), i.subject_id::text, c.status FROM adrient.access_review_items i
JOIN adrient.access_review_campaigns c ON c.id=i.campaign_id WHERE i.org_id::text=$1 AND i.id::text=$2`, orgID, itemID).Scan(&reviewer, &subject, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "open" {
		return fmt.Errorf("%w: the campaign is closed", ErrBad)
	}
	if subject == actor {
		return fmt.Errorf("%w: you cannot review your own access", ErrForbid)
	}
	if reviewer != actor && !isAdmin {
		return fmt.Errorf("%w: this review is assigned to someone else", ErrForbid)
	}
	if len(comment) > 500 {
		comment = comment[:500]
	}
	_, err = s.Pool.Exec(ctx, `UPDATE adrient.access_review_items SET decision=$3, decided_by=$4::uuid, decided_at=now(), comment=$5 WHERE id::text=$1 AND org_id::text=$2`,
		itemID, orgID, decision, actor, comment)
	return err
}

// Close ends a campaign and applies it: removals take effect, and items nobody
// answered follow the campaign's on_no_response setting.
func (s *Service) Close(ctx context.Context, orgID, actor, id string) (Campaign, error) {
	c, err := s.Get(ctx, orgID, id)
	if err != nil {
		return c, err
	}
	if c.Status == "closed" {
		return c, fmt.Errorf("%w: already closed", ErrBad)
	}
	if c.OnNoResponse == "remove" {
		_, _ = s.Pool.Exec(ctx, `UPDATE adrient.access_review_items SET decision='remove', comment='No response before the due date' WHERE campaign_id::text=$1 AND decision='pending'`, id)
	} else {
		_, _ = s.Pool.Exec(ctx, `UPDATE adrient.access_review_items SET decision='keep', comment='No response before the due date' WHERE campaign_id::text=$1 AND decision='pending'`, id)
	}
	items, err := s.Items(ctx, orgID, id)
	if err != nil {
		return c, err
	}
	removed := 0
	for _, it := range items {
		if it.Decision != "remove" || it.AppliedAt != nil {
			continue
		}
		res := s.apply(ctx, orgID, actor, it)
		_, _ = s.Pool.Exec(ctx, `UPDATE adrient.access_review_items SET applied_at=now(), apply_result=$2 WHERE id::text=$1`, it.ID, res)
		if !strings.HasPrefix(res, "failed") {
			removed++
		}
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE adrient.access_review_campaigns SET status='closed', closed_at=now() WHERE id::text=$1`, id)
	s.audit(ctx, orgID, actor, "review.close", map[string]any{"campaign": c.Name, "removed": removed})
	if s.Alert != nil {
		s.Alert(ctx, orgID, "review.completed", "Access review completed: "+c.Name, fmt.Sprintf("%d accesses were removed.", removed))
	}
	if c.RecurrenceDays != nil {
		next := CreateInput{Name: c.Name, Kind: c.Kind, DueDays: *c.RecurrenceDays, OnNoResponse: c.OnNoResponse, RecurrenceDays: c.RecurrenceDays}
		var sc struct {
			GroupIDs []string `json:"group_ids"`
		}
		_ = json.Unmarshal(c.Scope, &sc)
		next.GroupIDs = sc.GroupIDs
		_, _ = s.Create(ctx, orgID, actor, next)
	}
	return s.Get(ctx, orgID, id)
}

// apply carries out one removal and reports what happened.
func (s *Service) apply(ctx context.Context, orgID, actor string, it Item) string {
	switch it.ResourceType {
	case "group":
		tag, err := s.Pool.Exec(ctx, `UPDATE adrient.groups SET member_ids = member_ids - $3::text, updated_at=now() WHERE org_id::text=$1 AND id::text=$2`, orgID, it.ResourceID, it.SubjectID)
		if err != nil {
			return "failed: " + err.Error()
		}
		if tag.RowsAffected() == 0 {
			return "group no longer exists"
		}
		return "removed from the group"
	case "guest":
		if s.Suspend == nil {
			return "failed: no suspend action configured"
		}
		if err := s.Suspend(ctx, orgID, it.SubjectID, actor, "Access removed in review: "+it.Campaign); err != nil {
			return "failed: " + err.Error()
		}
		return "guest suspended"
	case "admin_role":
		if it.ResourceID == "admin" {
			_, _ = s.Pool.Exec(ctx, `DELETE FROM adrient.org_admins WHERE org_id::text=$1 AND user_id::text=$2`, orgID, it.SubjectID)
		}
		_, err := s.Pool.Exec(ctx, `DELETE FROM adrient.role_assignments ra USING adrient.roles r WHERE r.id=ra.role_id AND ra.org_id::text=$1 AND ra.subject_type='user' AND ra.subject_id::text=$2 AND r.key=$3`,
			orgID, it.SubjectID, it.ResourceID)
		if err != nil {
			return "failed: " + err.Error()
		}
		return "role removed"
	}
	return "failed: unknown item"
}

// CloseDue closes campaigns past their due date (run periodically).
func (s *Service) CloseDue(ctx context.Context) (int, error) {
	rows, err := s.Pool.Query(ctx, `SELECT org_id::text, id::text, COALESCE(created_by::text,'') FROM adrient.access_review_campaigns WHERE status='open' AND due_at < now()`)
	if err != nil {
		return 0, err
	}
	type due struct{ org, id, by string }
	var todo []due
	for rows.Next() {
		var d due
		if rows.Scan(&d.org, &d.id, &d.by) == nil {
			todo = append(todo, d)
		}
	}
	rows.Close()
	n := 0
	for _, d := range todo {
		if _, err := s.Close(ctx, d.org, d.by, d.id); err == nil {
			n++
		}
	}
	return n, nil
}

// Export returns a campaign's decisions as CSV for auditors.
func (s *Service) Export(ctx context.Context, orgID, id string) (string, error) {
	items, err := s.Items(ctx, orgID, id)
	if err != nil {
		return "", err
	}
	q := func(v string) string { return `"` + strings.ReplaceAll(v, `"`, `""`) + `"` }
	var b strings.Builder
	b.WriteString("person,e-mail,access,resource,reviewer,decision,comment,decided_at,applied\n")
	for _, it := range items {
		at := ""
		if it.DecidedAt != nil {
			at = it.DecidedAt.UTC().Format(time.RFC3339)
		}
		b.WriteString(strings.Join([]string{q(it.SubjectName), q(it.SubjectEmail), q(it.ResourceType), q(it.ResourceName), q(it.ReviewerName), q(it.Decision), q(it.Comment), q(at), q(it.ApplyResult)}, ",") + "\n")
	}
	return b.String(), nil
}

// ItemsPage is one window of a campaign's items (by subject email). q matches
// the subject email or the resource name; decision "pending" etc. filters.
func (s *Service) ItemsPage(ctx context.Context, orgID, campaignID, q, decision string, limit, offset int) ([]Item, error) {
	if limit <= 0 || limit > 501 {
		limit = 100
	}
	like := "%" + strings.ToLower(strings.TrimSpace(q)) + "%"
	rows, err := s.Pool.Query(ctx, `SELECT `+itemCols+itemFrom+` WHERE i.org_id::text=$1 AND i.campaign_id::text=$2
		AND ($3 = '%%' OR lower(su.email) LIKE $3 OR lower(i.resource_name) LIKE $3) AND ($4 = '' OR i.decision = $4)
		ORDER BY su.email, i.resource_name, i.id LIMIT $5 OFFSET $6`, orgID, campaignID, like, decision, limit, max(offset, 0))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
