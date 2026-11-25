package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// FeedTask is one open dated task for the calendar file.
type FeedTask struct {
	ID, Title, Notes, List, Link string
	Due                          time.Time
	UpdatedAt                    time.Time
	Recurrence                   string
	RemindMinutes                *int
}

// OpenDated returns the caller's open tasks that have a due date: tasks on
// lists they own or that are shared with them.
func (r *Repository) OpenDated(ctx context.Context, org, user string) ([]FeedTask, error) {
	rows, err := r.pool.Query(ctx, `SELECT t.id::text, t.title, t.notes, l.name, COALESCE(x.link_url,''), t.due_at, t.updated_at,
		COALESCE(x.recurrence,''), x.remind_minutes
		FROM adrient.tasks t JOIN adrient.task_lists l ON l.id=t.list_id
		LEFT JOIN adrient.task_extras x ON x.task_id=t.id
		WHERE t.org_id=$1 AND NOT t.completed AND t.due_at IS NOT NULL AND t.parent_task_id IS NULL
		AND (l.owner_user_id=$2 OR EXISTS (SELECT 1 FROM adrient.task_list_shares s WHERE s.list_id=l.id AND s.user_id=$2))
		ORDER BY t.due_at LIMIT 2000`, org, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FeedTask
	for rows.Next() {
		var f FeedTask
		if err := rows.Scan(&f.ID, &f.Title, &f.Notes, &f.List, &f.Link, &f.Due, &f.UpdatedAt, &f.Recurrence, &f.RemindMinutes); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// TagCount is a tag with the number of open tasks using it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// Tags returns every tag used by open tasks the caller can see.
func (r *Repository) Tags(ctx context.Context, org, user string) ([]TagCount, error) {
	rows, err := r.pool.Query(ctx, `SELECT tag, count(*) FROM (
		SELECT unnest(x.tags) AS tag FROM adrient.task_extras x JOIN adrient.tasks t ON t.id=x.task_id
		JOIN adrient.task_lists l ON l.id=t.list_id
		WHERE t.org_id=$1 AND NOT t.completed
		AND (l.owner_user_id=$2 OR EXISTS (SELECT 1 FROM adrient.task_list_shares s WHERE s.list_id=l.id AND s.user_id=$2))
	) q GROUP BY tag ORDER BY count(*) DESC, tag LIMIT 200`, org, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TagCount{}
	for rows.Next() {
		var c TagCount
		if err := rows.Scan(&c.Tag, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func icsEscape(s string) string {
	s = strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", "\r\n", "\\n", "\n", "\\n", "\r", "").Replace(s)
	return s
}

// icsFold folds a content line at 73 bytes on rune boundaries (RFC 5545).
func icsFold(line string) string {
	if len(line) <= 73 {
		return line
	}
	var b strings.Builder
	n := 0
	for _, r := range line {
		l := len(string(r))
		if n+l > 73 {
			b.WriteString("\r\n ")
			n = 1
		}
		b.WriteRune(r)
		n += l
	}
	return b.String()
}

func icsRRule(rule string) string {
	r, ok := ParseRule(rule)
	if !ok {
		return ""
	}
	freq := map[string]string{"daily": "DAILY", "weekly": "WEEKLY", "monthly": "MONTHLY", "yearly": "YEARLY"}[r.Freq]
	s := fmt.Sprintf("FREQ=%s;INTERVAL=%d", freq, r.Interval)
	if len(r.Weekdays) > 0 {
		names := []string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}
		var d []string
		for _, w := range r.Weekdays {
			d = append(d, names[w])
		}
		s += ";BYDAY=" + strings.Join(d, ",")
	}
	return s
}

// BuildICS renders open dated tasks as an iCalendar file: one 30-minute event
// per task with a stable UID, an optional alarm and RRULE.
func BuildICS(tasks []FeedTask, host string) string {
	var lines []string
	add := func(s string) { lines = append(lines, icsFold(s)) }
	add("BEGIN:VCALENDAR")
	add("VERSION:2.0")
	add("PRODID:-//Adrient Workspace//Tasks//EN")
	add("CALSCALE:GREGORIAN")
	add("X-WR-CALNAME:Adrient Tasks")
	stamp := time.Now().UTC().Format("20060102T150405Z")
	for _, t := range tasks {
		add("BEGIN:VEVENT")
		add("UID:task-" + t.ID + "@" + host)
		add("DTSTAMP:" + stamp)
		add("LAST-MODIFIED:" + t.UpdatedAt.UTC().Format("20060102T150405Z"))
		add("DTSTART:" + t.Due.UTC().Format("20060102T150405Z"))
		add("DTEND:" + t.Due.Add(30*time.Minute).UTC().Format("20060102T150405Z"))
		add("SUMMARY:" + icsEscape(t.Title))
		desc := t.Notes
		if t.Link != "" {
			desc = strings.TrimSpace(desc + "\n" + t.Link)
		}
		if desc != "" {
			add("DESCRIPTION:" + icsEscape(desc))
		}
		add("CATEGORIES:" + icsEscape(t.List))
		if rr := icsRRule(t.Recurrence); rr != "" {
			add("RRULE:" + rr)
		}
		if t.RemindMinutes != nil {
			add("BEGIN:VALARM")
			add("ACTION:DISPLAY")
			add("DESCRIPTION:" + icsEscape(t.Title))
			add(fmt.Sprintf("TRIGGER:-PT%dM", *t.RemindMinutes))
			add("END:VALARM")
		}
		add("END:VEVENT")
	}
	add("END:VCALENDAR")
	return strings.Join(lines, "\r\n") + "\r\n"
}
