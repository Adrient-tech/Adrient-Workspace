package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Roles on a list.
const (
	RoleOwner  = "owner"
	RoleEditor = "editor"
	RoleViewer = "viewer"
	RoleNone   = ""
)

// ErrInvalid marks bad input.
var ErrInvalid = errors.New("invalid input")

// ErrForbidden means the caller may not do this on the list.
var ErrForbidden = errors.New("forbidden")

// Extras are the extra attributes of a task.
type Extras struct {
	TaskID        string `json:"task_id"`
	AssigneeID    string `json:"assignee_id"`
	AssigneeEmail string `json:"assignee_email,omitempty"`
	AssigneeName  string `json:"assignee_name,omitempty"`
	Priority      int    `json:"priority"`
	Recurrence    string `json:"recurrence"`
	RemindMinutes *int   `json:"remind_minutes"`
	BoardStatus   string `json:"board_status"`
	LinkURL       string `json:"link_url"`
	// Tags: up to 5 per task, lower case (wave 6).
	Tags []string `json:"tags"`
}

// NormalizeTags lower-cases, de-duplicates and validates tags (max 5, 32 runes,
// letters, digits, "_" and "-"). A leading "#" is dropped.
func NormalizeTags(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		if t == "" || seen[t] {
			continue
		}
		if utf8.RuneCountInString(t) > 32 {
			return nil, fmt.Errorf("%w: tag too long", ErrInvalid)
		}
		for _, c := range t {
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' && c != '-' {
				return nil, fmt.Errorf("%w: tags may use letters, digits, _ and -", ErrInvalid)
			}
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > 5 {
		return nil, fmt.Errorf("%w: at most 5 tags", ErrInvalid)
	}
	return out, nil
}

var recurrences = map[string]bool{"": true, "daily": true, "weekdays": true, "weekly": true, "monthly": true, "yearly": true}

// Normalize validates the extras.
func (e *Extras) Normalize() error {
	if e.Priority < 0 || e.Priority > 3 {
		return fmt.Errorf("%w: priority must be 0 to 3", ErrInvalid)
	}
	if _, ok := ParseRule(e.Recurrence); !recurrences[e.Recurrence] && !ok {
		return fmt.Errorf("%w: unknown repeat rule", ErrInvalid)
	}
	tags, err := NormalizeTags(e.Tags)
	if err != nil {
		return err
	}
	e.Tags = tags
	if e.BoardStatus == "" {
		e.BoardStatus = "todo"
	}
	if e.BoardStatus != "todo" && e.BoardStatus != "doing" {
		return fmt.Errorf("%w: board status must be todo or doing", ErrInvalid)
	}
	if e.RemindMinutes != nil && (*e.RemindMinutes < 0 || *e.RemindMinutes > 60*24*14) {
		return fmt.Errorf("%w: reminder out of range", ErrInvalid)
	}
	e.LinkURL = strings.TrimSpace(e.LinkURL)
	if utf8.RuneCountInString(e.LinkURL) > 500 {
		return fmt.Errorf("%w: link too long", ErrInvalid)
	}
	if e.LinkURL != "" {
		l := strings.ToLower(e.LinkURL)
		if !strings.HasPrefix(l, "http://") && !strings.HasPrefix(l, "https://") && !strings.HasPrefix(l, "/") {
			return fmt.Errorf("%w: link must be a web address", ErrInvalid)
		}
	}
	return nil
}

// NextDue returns the due date of the next occurrence of a repeating task.
// Month ends are clamped (31 January + 1 month = 28 or 29 February) and the
// time of day is kept.
func NextDue(due time.Time, rule string) (time.Time, bool) {
	if r, ok := ParseRule(rule); ok && strings.HasPrefix(rule, "every:") {
		return r.nextAfter(due), true
	}
	switch rule {
	case "daily":
		return due.AddDate(0, 0, 1), true
	case "weekly":
		return due.AddDate(0, 0, 7), true
	case "weekdays":
		n := due.AddDate(0, 0, 1)
		for n.Weekday() == time.Saturday || n.Weekday() == time.Sunday {
			n = n.AddDate(0, 0, 1)
		}
		return n, true
	case "monthly":
		return addMonthsClamped(due, 1), true
	case "yearly":
		return addMonthsClamped(due, 12), true
	}
	return due, false
}

func addMonthsClamped(t time.Time, months int) time.Time {
	y, m, d := t.Date()
	first := time.Date(y, m+time.Month(months), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	last := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
	if d > last {
		d = last
	}
	return time.Date(first.Year(), first.Month(), d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}

// Role returns the caller's role on a list.
func (r *Repository) Role(ctx context.Context, org, user, listID string) (string, error) {
	var owner string
	err := r.pool.QueryRow(ctx, `SELECT owner_user_id::text FROM adrient.task_lists WHERE id=$1 AND org_id=$2`, listID, org).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return RoleNone, ErrNotFound
	}
	if err != nil {
		return RoleNone, err
	}
	if owner == user {
		return RoleOwner, nil
	}
	var role string
	err = r.pool.QueryRow(ctx, `SELECT role FROM adrient.task_list_shares WHERE list_id=$1 AND user_id=$2`, listID, user).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return RoleNone, nil
	}
	return role, err
}

// ListOfTask returns the list a task belongs to.
func (r *Repository) ListOfTask(ctx context.Context, org, taskID string) (string, error) {
	var l string
	err := r.pool.QueryRow(ctx, `SELECT list_id::text FROM adrient.tasks WHERE id=$1 AND org_id=$2`, taskID, org).Scan(&l)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return l, err
}

// Require checks the caller has at least the wanted role on a list.
func (r *Repository) Require(ctx context.Context, org, user, listID string, write bool) error {
	role, err := r.Role(ctx, org, user, listID)
	if err != nil {
		return err
	}
	switch {
	case role == RoleOwner, role == RoleEditor:
		return nil
	case role == RoleViewer && !write:
		return nil
	}
	return ErrForbidden
}

// RequireTask checks access through the task's list.
func (r *Repository) RequireTask(ctx context.Context, org, user, taskID string, write bool) (string, error) {
	l, err := r.ListOfTask(ctx, org, taskID)
	if err != nil {
		return "", err
	}
	return l, r.Require(ctx, org, user, l, write)
}

// GetExtras returns extras for every task of a list (only tasks that have some).
func (r *Repository) ListExtras(ctx context.Context, org, listID string) (map[string]Extras, error) {
	rows, err := r.pool.Query(ctx, `SELECT x.task_id::text, COALESCE(x.assignee_id::text,''), COALESCE(u.email,''), COALESCE(u.display_name,''),
		x.priority, x.recurrence, x.remind_minutes, x.board_status, x.link_url, x.tags
		FROM adrient.task_extras x JOIN adrient.tasks t ON t.id=x.task_id
		LEFT JOIN adrient.users u ON u.id=x.assignee_id
		WHERE t.list_id=$1 AND t.org_id=$2`, listID, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Extras{}
	for rows.Next() {
		var e Extras
		if err := rows.Scan(&e.TaskID, &e.AssigneeID, &e.AssigneeEmail, &e.AssigneeName, &e.Priority, &e.Recurrence, &e.RemindMinutes, &e.BoardStatus, &e.LinkURL, &e.Tags); err != nil {
			return nil, err
		}
		out[e.TaskID] = e
	}
	return out, rows.Err()
}

// SaveExtras stores extras. Assigning someone who has no access to the list
// shares it with them as editor. It returns whether the assignee changed.
func (r *Repository) SaveExtras(ctx context.Context, org, user, taskID string, e Extras) (assigneeChanged bool, err error) {
	if err := e.Normalize(); err != nil {
		return false, err
	}
	listID, err := r.RequireTask(ctx, org, user, taskID, true)
	if err != nil {
		return false, err
	}
	var prev *string
	_ = r.pool.QueryRow(ctx, `SELECT assignee_id::text FROM adrient.task_extras WHERE task_id=$1`, taskID).Scan(&prev)
	var assignee *string
	if e.AssigneeID != "" {
		var n int
		if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.users WHERE id=$1 AND org_id=$2`, e.AssigneeID, org).Scan(&n); err != nil || n == 0 {
			return false, fmt.Errorf("%w: assignee is not in your organisation", ErrInvalid)
		}
		assignee = &e.AssigneeID
		role, _ := r.Role(ctx, org, e.AssigneeID, listID)
		if role == RoleNone {
			if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.task_list_shares (list_id, org_id, user_id, role) VALUES ($1,$2,$3,'editor') ON CONFLICT DO NOTHING`, listID, org, e.AssigneeID); err != nil {
				return false, err
			}
		}
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.task_extras (task_id, org_id, assignee_id, priority, recurrence, remind_minutes, board_status, link_url, tags)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (task_id) DO UPDATE SET
		assignee_id=$3, priority=$4, recurrence=$5, remind_minutes=$6, board_status=$7, link_url=$8, tags=$9, updated_at=now()`,
		taskID, org, assignee, e.Priority, e.Recurrence, e.RemindMinutes, e.BoardStatus, e.LinkURL, e.Tags)
	if err != nil {
		return false, err
	}
	was := ""
	if prev != nil {
		was = *prev
	}
	return e.AssigneeID != "" && e.AssigneeID != was, nil
}

// AssignedTask is a task assigned to the caller, with its list.
type AssignedTask struct {
	TaskItem
	ListName string
	Extras   Extras
}

// Assigned returns open tasks assigned to the user, soonest due first.
func (r *Repository) Assigned(ctx context.Context, org, user string) ([]AssignedTask, error) {
	rows, err := r.pool.Query(ctx, `SELECT t.id::text, t.org_id::text, t.list_id::text, t.owner_user_id::text, t.title, t.notes, t.due_at, t.completed, t.completed_at,
		COALESCE(t.parent_task_id::text, ''), t.position, t.created_at, t.updated_at, l.name, x.priority, x.recurrence, x.remind_minutes, x.board_status, x.link_url
		FROM adrient.task_extras x JOIN adrient.tasks t ON t.id=x.task_id JOIN adrient.task_lists l ON l.id=t.list_id
		WHERE x.org_id=$1 AND x.assignee_id=$2 AND NOT t.completed
		ORDER BY t.due_at NULLS LAST, t.created_at LIMIT 200`, org, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AssignedTask
	for rows.Next() {
		var a AssignedTask
		if err := rows.Scan(&a.ID, &a.OrgID, &a.ListID, &a.OwnerUserID, &a.Title, &a.Notes, &a.DueAt, &a.Completed, &a.CompletedAt,
			&a.ParentTaskID, &a.Position, &a.CreatedAt, &a.UpdatedAt, &a.ListName, &a.Extras.Priority, &a.Extras.Recurrence,
			&a.Extras.RemindMinutes, &a.Extras.BoardStatus, &a.Extras.LinkURL); err != nil {
			return nil, err
		}
		a.Extras.TaskID = a.ID
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---- sharing ----

// Share is one person a list is shared with.
type Share struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

// Shares lists who a list is shared with (owner only).
func (r *Repository) Shares(ctx context.Context, org, user, listID string) ([]Share, error) {
	if role, err := r.Role(ctx, org, user, listID); err != nil {
		return nil, err
	} else if role != RoleOwner {
		return nil, ErrForbidden
	}
	rows, err := r.pool.Query(ctx, `SELECT s.user_id::text, u.email, u.display_name, s.role FROM adrient.task_list_shares s
		JOIN adrient.users u ON u.id=s.user_id WHERE s.list_id=$1 ORDER BY u.email`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Share{}
	for rows.Next() {
		var s Share
		if err := rows.Scan(&s.UserID, &s.Email, &s.Name, &s.Role); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// AddShare shares a list with an organisation member by email.
func (r *Repository) AddShare(ctx context.Context, org, user, listID, email, role string) (Share, error) {
	if role != RoleViewer && role != RoleEditor {
		return Share{}, fmt.Errorf("%w: role must be viewer or editor", ErrInvalid)
	}
	if r, err := r.Role(ctx, org, user, listID); err != nil {
		return Share{}, err
	} else if r != RoleOwner {
		return Share{}, ErrForbidden
	}
	var s Share
	err := r.pool.QueryRow(ctx, `SELECT id::text, email, display_name FROM adrient.users WHERE org_id=$1 AND lower(email)=lower($2)`,
		org, strings.TrimSpace(email)).Scan(&s.UserID, &s.Email, &s.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return Share{}, fmt.Errorf("%w: no one in your organisation has that email", ErrInvalid)
	}
	if err != nil {
		return Share{}, err
	}
	if s.UserID == user {
		return Share{}, fmt.Errorf("%w: you already own this list", ErrInvalid)
	}
	s.Role = role
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.task_list_shares (list_id, org_id, user_id, role) VALUES ($1,$2,$3,$4)
		ON CONFLICT (list_id, user_id) DO UPDATE SET role=$4`, listID, org, s.UserID, role)
	return s, err
}

// RemoveShare stops sharing with a user. A user may also remove themselves.
func (r *Repository) RemoveShare(ctx context.Context, org, user, listID, target string) error {
	role, err := r.Role(ctx, org, user, listID)
	if err != nil {
		return err
	}
	if role != RoleOwner && target != user {
		return ErrForbidden
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM adrient.task_list_shares WHERE list_id=$1 AND user_id=$2`, listID, target)
	return err
}

// SharedList is a list someone shared with the caller.
type SharedList struct {
	List
	OwnerName string
	Role      string
}

// SharedWithMe returns lists shared with the user.
func (r *Repository) SharedWithMe(ctx context.Context, org, user string) ([]SharedList, error) {
	rows, err := r.pool.Query(ctx, `SELECT l.id::text, l.org_id::text, l.owner_user_id::text, l.name, l.position, l.created_at, u.display_name, s.role
		FROM adrient.task_list_shares s JOIN adrient.task_lists l ON l.id=s.list_id JOIN adrient.users u ON u.id=l.owner_user_id
		WHERE s.org_id=$1 AND s.user_id=$2 ORDER BY l.name`, org, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SharedList{}
	for rows.Next() {
		var s SharedList
		if err := rows.Scan(&s.ID, &s.OrgID, &s.OwnerUserID, &s.Name, &s.Position, &s.CreatedAt, &s.OwnerName, &s.Role); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---- quick add (from Mail or Chat) ----

// Quick creates a task in the user's "Inbox" list (created on first use).
func (r *Repository) Quick(ctx context.Context, org, user, title, notes, link string, due *time.Time) (TaskItem, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return TaskItem{}, fmt.Errorf("%w: a title is required", ErrInvalid)
	}
	if utf8.RuneCountInString(title) > 300 {
		title = string([]rune(title)[:300])
	}
	if utf8.RuneCountInString(notes) > 8000 {
		notes = string([]rune(notes)[:8000])
	}
	e := Extras{LinkURL: link, BoardStatus: "todo"}
	if err := e.Normalize(); err != nil {
		return TaskItem{}, err
	}
	lists, err := r.ListLists(ctx, org, user)
	if err != nil {
		return TaskItem{}, err
	}
	listID := ""
	for _, l := range lists {
		if l.Name == "Inbox" {
			listID = l.ID
			break
		}
	}
	if listID == "" {
		l, err := r.CreateList(ctx, org, user, ListFields{Name: "Inbox"})
		if err != nil {
			return TaskItem{}, err
		}
		listID = l.ID
	}
	t, err := r.CreateTask(ctx, org, listID, user, TaskFields{Title: title, Notes: notes, DueAt: due})
	if err != nil {
		return TaskItem{}, err
	}
	if link != "" {
		_, err = r.pool.Exec(ctx, `INSERT INTO adrient.task_extras (task_id, org_id, link_url) VALUES ($1,$2,$3)`, t.ID, org, e.LinkURL)
	}
	return t, err
}

// ---- recurrence on completion ----

// SpawnNext creates the next occurrence of a repeating task that was just
// completed. It does nothing for tasks without a repeat rule, and never twice
// for the same completion.
func (r *Repository) SpawnNext(ctx context.Context, done TaskItem) (*TaskItem, error) {
	if !done.Completed {
		return nil, nil
	}
	var e Extras
	var assignee *string
	err := r.pool.QueryRow(ctx, `SELECT assignee_id::text, priority, recurrence, remind_minutes, link_url, tags FROM adrient.task_extras WHERE task_id=$1`, done.ID).
		Scan(&assignee, &e.Priority, &e.Recurrence, &e.RemindMinutes, &e.LinkURL, &e.Tags)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && e.Recurrence == "") {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	base := time.Now()
	if done.DueAt != nil {
		base = *done.DueAt
	}
	next, ok := NextDue(base, e.Recurrence)
	if !ok {
		return nil, nil
	}
	if rule, isRule := ParseRule(e.Recurrence); isRule && strings.HasPrefix(e.Recurrence, "every:") {
		next = rule.Next(base, time.Now())
	}
	// A late completion should not schedule the next one in the past.
	for next.Before(time.Now()) && done.DueAt != nil {
		n, _ := NextDue(next, e.Recurrence)
		if !n.After(next) {
			break
		}
		next = n
	}
	// Clear the rule on the finished task so toggling it back and forth cannot spawn more.
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.task_extras SET recurrence='' WHERE task_id=$1 AND recurrence<>''`, done.ID)
	if err != nil || tag.RowsAffected() == 0 {
		return nil, err
	}
	t, err := r.CreateTask(ctx, done.OrgID, done.ListID, done.OwnerUserID, TaskFields{Title: done.Title, Notes: done.Notes, DueAt: &next, ParentTaskID: done.ParentTaskID})
	if err != nil {
		return nil, err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.task_extras (task_id, org_id, assignee_id, priority, recurrence, remind_minutes, link_url, tags)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, t.ID, done.OrgID, assignee, e.Priority, e.Recurrence, e.RemindMinutes, e.LinkURL, e.Tags)
	return &t, err
}

// ---- reminders ----

// Notifier delivers a notification.
type Notifier func(ctx context.Context, org, user, title, body, target string) error

type dueTask struct {
	ID, Org, Title, Owner, Assignee string
	Due                             time.Time
	Minutes                         int
}

// remindersDue picks tasks whose reminder time passed within lookback and that are still open and not yet due long ago.
func remindersDue(rows []dueTask, now time.Time, lookback time.Duration) []dueTask {
	var out []dueTask
	for _, t := range rows {
		fire := t.Due.Add(-time.Duration(t.Minutes) * time.Minute)
		if !fire.After(now) && fire.After(now.Add(-lookback)) {
			out = append(out, t)
		}
	}
	return out
}

// RunReminders sends due task reminders once, recording each before sending.
func (r *Repository) RunReminders(ctx context.Context, now time.Time, notify Notifier) (int, error) {
	rows, err := r.pool.Query(ctx, `SELECT t.id::text, t.org_id::text, t.title, t.owner_user_id::text, COALESCE(x.assignee_id::text,''), t.due_at, x.remind_minutes
		FROM adrient.tasks t JOIN adrient.task_extras x ON x.task_id=t.id
		WHERE NOT t.completed AND t.due_at IS NOT NULL AND x.remind_minutes IS NOT NULL
		  AND t.due_at BETWEEN $1 AND $2 LIMIT 5000`, now.Add(-time.Hour), now.Add(15*24*time.Hour))
	if err != nil {
		return 0, err
	}
	var all []dueTask
	for rows.Next() {
		var d dueTask
		if err := rows.Scan(&d.ID, &d.Org, &d.Title, &d.Owner, &d.Assignee, &d.Due, &d.Minutes); err != nil {
			rows.Close()
			return 0, err
		}
		all = append(all, d)
	}
	rows.Close()
	sent := 0
	for _, d := range remindersDue(all, now, 30*time.Minute) {
		who := d.Owner
		if d.Assignee != "" {
			who = d.Assignee
		}
		tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.task_reminders_sent (task_id, due_at, user_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, d.ID, d.Due, who)
		if err != nil || tag.RowsAffected() == 0 {
			continue
		}
		body := "Due " + d.Due.UTC().Format("2 Jan 15:04 UTC")
		if notify != nil && notify(ctx, d.Org, who, "Task: "+d.Title, body, "/tasks") == nil {
			sent++
		}
	}
	return sent, nil
}
