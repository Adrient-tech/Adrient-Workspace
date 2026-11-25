package contacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Extras are the extra details kept per contact.
type Extras struct {
	ContactID   string `json:"contact_id"`
	Birthday    string `json:"birthday"`    // YYYY-MM-DD or ""
	Anniversary string `json:"anniversary"` // YYYY-MM-DD or ""
	Website     string `json:"website"`
	Address     string `json:"address"`
	Nickname    string `json:"nickname"`
}

// ErrInvalid marks bad input.
var ErrInvalid = errors.New("invalid input")

func parseDay(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil || t.Year() < 1900 || t.Year() > 2100 {
		return nil, fmt.Errorf("%w: dates must look like 1990-05-17", ErrInvalid)
	}
	return &t, nil
}

// Normalize validates the extras.
func (e *Extras) Normalize() error {
	if _, err := parseDay(e.Birthday); err != nil {
		return err
	}
	if _, err := parseDay(e.Anniversary); err != nil {
		return err
	}
	e.Website = strings.TrimSpace(e.Website)
	if e.Website != "" {
		l := strings.ToLower(e.Website)
		if !strings.HasPrefix(l, "http://") && !strings.HasPrefix(l, "https://") {
			e.Website = "https://" + e.Website
		}
	}
	if utf8.RuneCountInString(e.Website) > 300 || utf8.RuneCountInString(e.Address) > 500 || utf8.RuneCountInString(e.Nickname) > 100 {
		return fmt.Errorf("%w: a field is too long", ErrInvalid)
	}
	return nil
}

// GetExtras returns the extras of one contact (empty when none are saved).
func (r *Repository) GetExtras(ctx context.Context, org, contactID string) (Extras, error) {
	if _, err := r.Get(ctx, org, contactID); err != nil {
		return Extras{}, err
	}
	e := Extras{ContactID: contactID}
	var b, a *time.Time
	err := r.pool.QueryRow(ctx, `SELECT birthday, anniversary, website, address, nickname FROM adrient.contact_extras
		WHERE contact_id=$1 AND org_id=$2`, contactID, org).Scan(&b, &a, &e.Website, &e.Address, &e.Nickname)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return e, err
	}
	if b != nil {
		e.Birthday = b.Format("2006-01-02")
	}
	if a != nil {
		e.Anniversary = a.Format("2006-01-02")
	}
	return e, nil
}

// SaveExtras stores the extras of one contact.
func (r *Repository) SaveExtras(ctx context.Context, org, contactID string, e Extras) error {
	if err := e.Normalize(); err != nil {
		return err
	}
	if _, err := r.Get(ctx, org, contactID); err != nil {
		return err
	}
	b, _ := parseDay(e.Birthday)
	a, _ := parseDay(e.Anniversary)
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.contact_extras (contact_id, org_id, birthday, anniversary, website, address, nickname)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (contact_id) DO UPDATE SET
		birthday=$3, anniversary=$4, website=$5, address=$6, nickname=$7, updated_at=now()`,
		contactID, org, b, a, e.Website, e.Address, e.Nickname)
	return err
}

// DayEvent is a yearly date from a contact shown on the calendar.
type DayEvent struct {
	ContactID string `json:"contact_id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // birthday | anniversary
	Date      string `json:"date"` // the occurrence, YYYY-MM-DD
	Years     int    `json:"years,omitempty"`
}

// occurrences returns the yearly repeats of a date that fall in [from, to).
// A 29 February date is shown on 28 February in non-leap years.
func occurrences(d time.Time, from, to time.Time) []time.Time {
	var out []time.Time
	for y := from.Year(); y <= to.Year(); y++ {
		m, day := d.Month(), d.Day()
		if m == time.February && day == 29 && !isLeap(y) {
			day = 28
		}
		t := time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
		if y >= d.Year() && !t.Before(from) && t.Before(to) {
			out = append(out, t)
		}
	}
	return out
}

func isLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

// DayEvents lists the owner's contact birthdays and anniversaries in [from, to).
func (r *Repository) DayEvents(ctx context.Context, org, owner string, from, to time.Time) ([]DayEvent, error) {
	if to.Sub(from) > 400*24*time.Hour {
		return nil, fmt.Errorf("%w: window too large", ErrInvalid)
	}
	rows, err := r.pool.Query(ctx, `SELECT c.id::text, c.display_name, x.birthday, x.anniversary
		FROM adrient.contact_extras x JOIN adrient.contacts c ON c.id = x.contact_id
		WHERE x.org_id=$1 AND c.owner_id=$2 AND c.trashed_at IS NULL AND (x.birthday IS NOT NULL OR x.anniversary IS NOT NULL)`, org, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DayEvent{}
	for rows.Next() {
		var id, name string
		var b, a *time.Time
		if err := rows.Scan(&id, &name, &b, &a); err != nil {
			return nil, err
		}
		for kind, d := range map[string]*time.Time{"birthday": b, "anniversary": a} {
			if d == nil {
				continue
			}
			for _, t := range occurrences(*d, from, to) {
				out = append(out, DayEvent{ContactID: id, Name: name, Kind: kind, Date: t.Format("2006-01-02"), Years: t.Year() - d.Year()})
			}
		}
	}
	return out, rows.Err()
}

// ---- change journal ("Undo changes") ----

// journal records the contact as it is right now, before a change. Failures
// never block the change itself.
func (r *Repository) journal(ctx context.Context, org, id, action string) {
	if _, err := r.pool.Exec(ctx, `INSERT INTO adrient.contact_history (org_id, contact_id, owner_id, action, snapshot)
		SELECT org_id, id, owner_id, $3::text, to_jsonb(c) - 'trashed_at'
		FROM adrient.contacts c WHERE id=$1 AND org_id=$2 AND trashed_at IS NULL`, id, org, action); err != nil {
		slog.Warn("contacts: journal failed", "err", err)
	}
}

// HistoryItem is one undoable change.
type HistoryItem struct {
	ID        string    `json:"id"`
	ContactID string    `json:"contact_id"`
	Name      string    `json:"name"`
	Action    string    `json:"action"`
	At        time.Time `json:"at"`
	Undone    bool      `json:"undone"`
}

// UndoWindow is how long a change can be undone.
const UndoWindow = 30 * 24 * time.Hour

// History lists the owner's recent changes, newest first.
func (r *Repository) History(ctx context.Context, org, owner string) ([]HistoryItem, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, contact_id::text, COALESCE(snapshot->>'display_name',''), action, created_at, undone_at IS NOT NULL
		FROM adrient.contact_history WHERE org_id=$1 AND owner_id=$2 AND created_at > $3
		ORDER BY created_at DESC LIMIT 50`, org, owner, time.Now().Add(-UndoWindow))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryItem{}
	for rows.Next() {
		var h HistoryItem
		if err := rows.Scan(&h.ID, &h.ContactID, &h.Name, &h.Action, &h.At, &h.Undone); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Undo restores a contact to the state saved before the change.
func (r *Repository) Undo(ctx context.Context, org, owner, historyID string) error {
	var cid, action string
	var snap []byte
	var undone *time.Time
	var at time.Time
	err := r.pool.QueryRow(ctx, `SELECT contact_id::text, action, snapshot, undone_at, created_at FROM adrient.contact_history
		WHERE id=$1 AND org_id=$2 AND owner_id=$3`, historyID, org, owner).Scan(&cid, &action, &snap, &undone, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if undone != nil {
		return fmt.Errorf("%w: already undone", ErrInvalid)
	}
	if time.Since(at) > UndoWindow {
		return fmt.Errorf("%w: too old to undo", ErrInvalid)
	}
	var s struct {
		DisplayName string          `json:"display_name"`
		FirstName   string          `json:"first_name"`
		LastName    string          `json:"last_name"`
		Company     string          `json:"company"`
		JobTitle    string          `json:"job_title"`
		Emails      json.RawMessage `json:"emails"`
		Phones      json.RawMessage `json:"phones"`
		Labels      json.RawMessage `json:"labels"`
		Notes       string          `json:"notes"`
		Starred     bool            `json:"starred"`
	}
	if err := json.Unmarshal(snap, &s); err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE adrient.contacts SET display_name=$3, first_name=$4, last_name=$5, company=$6, job_title=$7,
		emails=$8, phones=$9, labels=$10, notes=$11, starred=$12, trashed_at=NULL, updated_at=now()
		WHERE id=$1 AND org_id=$2`, cid, org, s.DisplayName, s.FirstName, s.LastName, s.Company, s.JobTitle,
		nz(s.Emails), nz(s.Phones), nz(s.Labels), s.Notes, s.Starred)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `UPDATE adrient.contact_history SET undone_at=now() WHERE id=$1`, historyID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func nz(b json.RawMessage) []byte {
	if len(b) == 0 || string(b) == "null" {
		return []byte("[]")
	}
	return b
}

// ---- links to Calendar and CRM ----

// LinkedEvent is an upcoming meeting that includes the contact.
type LinkedEvent struct {
	ID    string    `json:"id"`
	Title string    `json:"title"`
	Start time.Time `json:"start"`
}

// LinkedDeal is a CRM deal tied to the contact.
type LinkedDeal struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Stage    string  `json:"stage"`
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

// Links is what other apps know about a contact.
type Links struct {
	Events    []LinkedEvent `json:"events"`
	CRMStatus string        `json:"crm_status"` // "" when the contact is not in CRM
	Deals     []LinkedDeal  `json:"deals"`
}

// LinksFor finds upcoming calendar events with the contact's emails as guests,
// and the contact's CRM status and deals.
func (r *Repository) LinksFor(ctx context.Context, org, contactID string) (Links, error) {
	c, err := r.Get(ctx, org, contactID)
	if err != nil {
		return Links{}, err
	}
	l := Links{Events: []LinkedEvent{}, Deals: []LinkedDeal{}}
	if len(c.Emails) > 0 {
		emails := make([]string, 0, len(c.Emails))
		for _, e := range c.Emails {
			emails = append(emails, strings.ToLower(e))
		}
		rows, err := r.pool.Query(ctx, `SELECT e.id::text, e.title, e.start_at FROM adrient.calendar_events e
			WHERE e.org_id=$1 AND e.trashed_at IS NULL AND e.recurrence_parent_id IS NULL AND e.start_at > now()
			  AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(e.attendees) a WHERE lower(a) = ANY($2))
			ORDER BY e.start_at LIMIT 10`, org, emails)
		if err != nil {
			return l, err
		}
		for rows.Next() {
			var ev LinkedEvent
			if err := rows.Scan(&ev.ID, &ev.Title, &ev.Start); err != nil {
				rows.Close()
				return l, err
			}
			l.Events = append(l.Events, ev)
		}
		rows.Close()
	}
	err = r.pool.QueryRow(ctx, `SELECT status FROM adrient.crm_people WHERE org_id=$1 AND contact_id=$2`, org, contactID).Scan(&l.CRMStatus)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return l, nil // CRM tables may not exist on a trimmed install; links are optional
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text, title, stage, value::float8, currency FROM adrient.crm_deals
		WHERE org_id=$1 AND contact_id=$2 ORDER BY updated_at DESC LIMIT 10`, org, contactID)
	if err == nil {
		for rows.Next() {
			var d LinkedDeal
			if rows.Scan(&d.ID, &d.Title, &d.Stage, &d.Value, &d.Currency) == nil {
				l.Deals = append(l.Deals, d)
			}
		}
		rows.Close()
	}
	return l, nil
}
