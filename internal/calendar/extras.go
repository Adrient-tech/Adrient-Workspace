package calendar

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Settings are a user's calendar preferences.
type Settings struct {
	Hours WorkingHours `json:"working_hours"`
}

// GetSettings returns the user's settings, or defaults when none are saved.
func (r *Repository) GetSettings(ctx context.Context, org, user string) (Settings, error) {
	var tz string
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT timezone, working_hours FROM adrient.calendar_settings WHERE org_id=$1 AND user_id=$2`, org, user).Scan(&tz, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{Hours: DefaultWorkingHours()}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	w := WorkingHours{Timezone: tz}
	_ = json.Unmarshal(raw, &w.Days)
	if w.Days == nil {
		w.Days = map[string][]HoursWindow{}
	}
	return Settings{Hours: w}, nil
}

// SaveSettings validates and stores the user's settings.
func (r *Repository) SaveSettings(ctx context.Context, org, user string, s Settings) error {
	if err := s.Hours.Validate(); err != nil {
		return err
	}
	days, _ := json.Marshal(s.Hours.Days)
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.calendar_settings (org_id, user_id, timezone, working_hours)
		VALUES ($1,$2,$3,$4) ON CONFLICT (org_id, user_id) DO UPDATE SET timezone=$3, working_hours=$4, updated_at=now()`,
		org, user, s.Hours.Timezone, days)
	return err
}

// OrgUser is a directory entry.
type OrgUser struct {
	ID, Email, Name string
}

// UsersByEmail resolves org members by email (case-insensitive).
func (r *Repository) UsersByEmail(ctx context.Context, org string, emails []string) ([]OrgUser, error) {
	if len(emails) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, email, display_name FROM adrient.users
		WHERE org_id=$1 AND lower(email) = ANY($2)`, org, lowerAll(emails))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrgUser
	for rows.Next() {
		var u OrgUser
		if err := rows.Scan(&u.ID, &u.Email, &u.Name); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(strings.TrimSpace(s))
	}
	return out
}

// FindTime suggests times when all listed people (plus the caller) are free.
// People who are not org members are treated as always free with default hours
// in UTC, so they never block a suggestion. The second return lists emails
// that could not be matched to an org member.
func (r *Repository) FindTime(ctx context.Context, org, caller, callerEmail string, emails []string, from, to time.Time, dur, step time.Duration, limit int) ([]Suggestion, []string, error) {
	all := append([]string{callerEmail}, emails...)
	seen := map[string]bool{}
	var uniq []string
	for _, e := range lowerAll(all) {
		if e != "" && !seen[e] {
			seen[e] = true
			uniq = append(uniq, e)
		}
	}
	if len(uniq) > 20 {
		return nil, nil, errors.New("at most 20 people")
	}
	if to.Sub(from) > 31*24*time.Hour {
		return nil, nil, errors.New("search window is limited to 31 days")
	}
	users, err := r.UsersByEmail(ctx, org, uniq)
	if err != nil {
		return nil, nil, err
	}
	byEmail := map[string]OrgUser{}
	for _, u := range users {
		byEmail[strings.ToLower(u.Email)] = u
	}
	var people []Person
	var unknown []string
	for _, e := range uniq {
		u, ok := byEmail[e]
		if !ok {
			unknown = append(unknown, e)
			continue
		}
		set, err := r.GetSettings(ctx, org, u.ID)
		if err != nil {
			return nil, nil, err
		}
		busy, err := r.BusyFor(ctx, org, u.ID, u.Email, from, to)
		if err != nil {
			return nil, nil, err
		}
		people = append(people, Person{Email: u.Email, Busy: busy, Hours: set.Hours})
	}
	return FindTimes(people, from, to, dur, step, limit), unknown, nil
}

// ListOwned returns the user's own non-exception events for export.
func (r *Repository) ListOwned(ctx context.Context, org, owner string) ([]Event, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+columns+` FROM adrient.calendar_events
		WHERE org_id=$1 AND owner_id=$2 AND trashed_at IS NULL AND recurrence_parent_id IS NULL
		ORDER BY start_at LIMIT 5000`, org, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ImportResult summarises an ICS import.
type ImportResult struct {
	Imported   int `json:"imported"`
	Duplicates int `json:"duplicates"`
	Skipped    int `json:"skipped"`
}

// ImportICS stores parsed events for the owner. Events whose UID was already
// imported by this user are skipped, so re-importing a file is harmless.
func (r *Repository) ImportICS(ctx context.Context, org, owner string, events []ICSEvent, skipped int) (ImportResult, error) {
	res := ImportResult{Skipped: skipped}
	for _, ev := range events {
		if ev.UID != "" {
			var n int
			if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.calendar_events
				WHERE org_id=$1 AND owner_id=$2 AND ics_uid=$3 AND trashed_at IS NULL`, org, owner, ev.UID).Scan(&n); err != nil {
				return res, err
			}
			if n > 0 {
				res.Duplicates++
				continue
			}
		}
		title := strings.TrimSpace(ev.Title)
		if title == "" {
			title = "(no title)"
		}
		if len(title) > 300 {
			title = title[:300]
		}
		created, err := r.Create(ctx, org, owner, Fields{Title: title, Description: capStr(ev.Description, 8000), Location: capStr(ev.Location, 500),
			StartAt: ev.Start, EndAt: ev.End, AllDay: ev.AllDay, Recurrence: ev.Recurrence,
			ItemType: ItemTypeEvent, Status: StatusBusy, Visibility: VisibilityDefault})
		if err != nil {
			res.Skipped++
			continue
		}
		if ev.UID != "" {
			if _, err := r.pool.Exec(ctx, `UPDATE adrient.calendar_events SET ics_uid=$2 WHERE id=$1`, created.ID, ev.UID); err != nil {
				return res, err
			}
		}
		res.Imported++
	}
	return res, nil
}

func capStr(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// FeedToken returns the user's private feed token, creating it on first use.
// With rotate it replaces the token, which stops old subscriptions.
func (r *Repository) FeedToken(ctx context.Context, org, user string, rotate bool) (string, error) {
	if !rotate {
		var t string
		err := r.pool.QueryRow(ctx, `SELECT token FROM adrient.calendar_feed_tokens WHERE user_id=$1`, user).Scan(&t)
		if err == nil {
			return t, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := hex.EncodeToString(b)
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.calendar_feed_tokens (user_id, org_id, token) VALUES ($1,$2,$3)
		ON CONFLICT (user_id) DO UPDATE SET token=$3, created_at=now()`, user, org, t)
	return t, err
}

// FeedByToken resolves a feed token to its owner and org.
func (r *Repository) FeedByToken(ctx context.Context, token string) (org, user, name string, err error) {
	if len(token) < 32 {
		return "", "", "", ErrNotFound
	}
	err = r.pool.QueryRow(ctx, `SELECT t.org_id::text, t.user_id::text, u.display_name
		FROM adrient.calendar_feed_tokens t JOIN adrient.users u ON u.id=t.user_id WHERE t.token=$1`, token).Scan(&org, &user, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	return
}

// ---- reminders ----

// DueReminder is a reminder that should fire now.
type DueReminder struct {
	Event      Event
	Occurrence time.Time
	Minutes    int32
}

// dueReminders picks reminders whose fire time passed within lookback and whose
// event has not started yet. Pure, so it is unit tested.
func dueReminders(masters []Event, now time.Time, lookback time.Duration) []DueReminder {
	var out []DueReminder
	horizon := now.Add(7 * 24 * time.Hour) // longest reminder offered is a week
	for _, m := range masters {
		if len(m.Reminders) == 0 || m.RecurrenceParentID != "" {
			continue
		}
		for _, inst := range expandEvent(m, now, horizon) {
			if !inst.StartAt.After(now) {
				continue
			}
			for _, mins := range m.Reminders {
				fire := inst.StartAt.Add(-time.Duration(mins) * time.Minute)
				if !fire.After(now) && fire.After(now.Add(-lookback)) {
					out = append(out, DueReminder{Event: inst, Occurrence: inst.StartAt, Minutes: mins})
				}
			}
		}
	}
	return out
}

// Notifier delivers a reminder to a user (wired to the notifications module).
type Notifier func(ctx context.Context, org, user, title, body, target string) error

// RunReminders fires due reminders once. A reminder is recorded before it is
// sent, so it is never delivered twice even across restarts or instances.
func (r *Repository) RunReminders(ctx context.Context, now time.Time, notify Notifier) (int, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+columns+` FROM adrient.calendar_events
		WHERE trashed_at IS NULL AND recurrence_parent_id IS NULL AND reminders <> '[]'::jsonb AND item_type <> 'out_of_office'
		  AND ((recurrence = '' AND start_at > $1 AND start_at < $2) OR (recurrence <> '' AND start_at < $2))
		LIMIT 5000`, now, now.Add(8*24*time.Hour))
	if err != nil {
		return 0, err
	}
	var masters []Event
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			rows.Close()
			return 0, err
		}
		masters = append(masters, e)
	}
	rows.Close()
	sent := 0
	for _, d := range dueReminders(masters, now, 15*time.Minute) {
		recips := []string{d.Event.OwnerID}
		if len(d.Event.Attendees) > 0 {
			us, err := r.UsersByEmail(ctx, d.Event.OrgID, d.Event.Attendees)
			if err == nil {
				for _, u := range us {
					if u.ID != d.Event.OwnerID {
						recips = append(recips, u.ID)
					}
				}
			}
		}
		masterID := d.Event.RecurringEventID
		if masterID == "" {
			masterID = d.Event.ID
		}
		for _, uid := range recips {
			tag, err := r.pool.Exec(ctx, `INSERT INTO adrient.calendar_reminders_sent (event_id, occurrence, minutes, user_id)
				VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, masterID, d.Occurrence, d.Minutes, uid)
			if err != nil || tag.RowsAffected() == 0 {
				continue
			}
			body := fmt.Sprintf("Starts at %s", d.Occurrence.UTC().Format("15:04 UTC"))
			if d.Minutes > 0 {
				body = fmt.Sprintf("Starts in %d minutes", d.Minutes)
			}
			if notify != nil && notify(ctx, d.Event.OrgID, uid, d.Event.Title, body, "/calendar") == nil {
				sent++
			}
		}
	}
	return sent, nil
}
