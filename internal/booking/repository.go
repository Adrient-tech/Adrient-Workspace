package booking

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned to callers (mapped to HTTP statuses by the handlers).
var (
	ErrNotFound    = errors.New("not found")
	ErrInvalid     = errors.New("invalid input")
	ErrSlotTaken   = errors.New("that time is no longer available")
	ErrNotBookable = errors.New("that time is not offered")
)

// Question is an extra question the guest answers when booking.
type Question struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Type     string `json:"type"` // text | longtext
	Required bool   `json:"required"`
}

// Page is a booking page (appointment schedule).
type Page struct {
	ID              string       `json:"id"`
	OrgID           string       `json:"-"`
	OwnerID         string       `json:"-"`
	Slug            string       `json:"slug"`
	Title           string       `json:"title"`
	Description     string       `json:"description"`
	Location        string       `json:"location"`
	DurationMin     int          `json:"duration_min"`
	BufferBeforeMin int          `json:"buffer_before_min"`
	BufferAfterMin  int          `json:"buffer_after_min"`
	MinNoticeMin    int          `json:"min_notice_min"`
	MaxDaysAhead    int          `json:"max_days_ahead"`
	SlotStepMin     int          `json:"slot_step_min"`
	MaxPerDay       int          `json:"max_per_day"`
	Timezone        string       `json:"timezone"`
	Availability    Availability `json:"availability"`
	Questions       []Question   `json:"questions"`
	CreateMeet      bool         `json:"create_meet"`
	Active          bool         `json:"active"`
}

// Appointment is one booking made through a page.
type Appointment struct {
	ID          string            `json:"id"`
	PageID      string            `json:"page_id"`
	PageTitle   string            `json:"page_title,omitempty"`
	OrgID       string            `json:"-"`
	OwnerID     string            `json:"-"`
	EventID     string            `json:"event_id,omitempty"`
	StartAt     time.Time         `json:"start_at"`
	EndAt       time.Time         `json:"end_at"`
	GuestName   string            `json:"guest_name"`
	GuestEmail  string            `json:"guest_email"`
	GuestTZ     string            `json:"guest_tz"`
	Answers     map[string]string `json:"answers"`
	MeetCode    string            `json:"meet_code,omitempty"`
	Status      string            `json:"status"`
	CancelToken string            `json:"-"`
}

// Deps connects the booking module to the rest of the suite.
type Deps struct {
	// Busy returns the owner's busy spans (Calendar).
	Busy func(ctx context.Context, org, owner, ownerEmail string, from, to time.Time) ([]Span, error)
	// CreateEvent puts the appointment in the owner's calendar, optionally with a Meet room.
	CreateEvent func(ctx context.Context, org, owner string, ev EventInput) (eventID, meetCode string, err error)
	// DeleteEvent removes a calendar event.
	DeleteEvent func(ctx context.Context, org, eventID string) error
	// Mail sends an email (nil when mail is not configured).
	Mail func(ctx context.Context, to []string, subject, html, text, icsName string, ics []byte) error
	// Owner returns the owner's display name and email.
	Owner func(ctx context.Context, org, owner string) (name, email string, err error)
	// BaseURL is the public origin used in links (no trailing slash).
	BaseURL string
	Now     func() time.Time
}

// EventInput describes the calendar event for an appointment.
type EventInput struct {
	Title, Description, Location string
	Start, End                   time.Time
	GuestEmail                   string
	Meet                         bool
}

// Repository stores booking pages and appointments.
type Repository struct {
	pool *pgxpool.Pool
	Deps Deps
}

// NewRepository builds a repository on pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, Deps: Deps{Now: time.Now}}
}

func (r *Repository) now() time.Time {
	if r.Deps.Now != nil {
		return r.Deps.Now()
	}
	return time.Now()
}

const pageCols = `id::text, org_id::text, owner_id::text, slug, title, description, location, duration_min,
	buffer_before_min, buffer_after_min, min_notice_min, max_days_ahead, slot_step_min, max_per_day, timezone,
	availability, questions, create_meet, active`

func scanPage(row pgx.Row) (Page, error) {
	var p Page
	var av, qs []byte
	err := row.Scan(&p.ID, &p.OrgID, &p.OwnerID, &p.Slug, &p.Title, &p.Description, &p.Location, &p.DurationMin,
		&p.BufferBeforeMin, &p.BufferAfterMin, &p.MinNoticeMin, &p.MaxDaysAhead, &p.SlotStepMin, &p.MaxPerDay,
		&p.Timezone, &av, &qs, &p.CreateMeet, &p.Active)
	if err != nil {
		return Page{}, err
	}
	_ = json.Unmarshal(av, &p.Availability)
	_ = json.Unmarshal(qs, &p.Questions)
	if p.Availability == nil {
		p.Availability = Availability{}
	}
	if p.Questions == nil {
		p.Questions = []Question{}
	}
	return p, nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify makes a URL-safe slug from a title.
func Slugify(s string) string {
	s = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-")
	}
	if s == "" {
		s = "meeting"
	}
	return s
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Normalize validates a page and fills defaults. It never trusts the client.
func (p *Page) Normalize() error {
	p.Title = strings.TrimSpace(p.Title)
	if p.Title == "" || utf8.RuneCountInString(p.Title) > 120 {
		return fmt.Errorf("%w: title is required (max 120 characters)", ErrInvalid)
	}
	if utf8.RuneCountInString(p.Description) > 2000 || utf8.RuneCountInString(p.Location) > 200 {
		return fmt.Errorf("%w: description or location too long", ErrInvalid)
	}
	if p.DurationMin < 5 || p.DurationMin > 480 {
		return fmt.Errorf("%w: duration must be 5 to 480 minutes", ErrInvalid)
	}
	if p.BufferBeforeMin < 0 || p.BufferBeforeMin > 240 || p.BufferAfterMin < 0 || p.BufferAfterMin > 240 {
		return fmt.Errorf("%w: buffers must be 0 to 240 minutes", ErrInvalid)
	}
	if p.MinNoticeMin < 0 || p.MinNoticeMin > 60*24*60 {
		return fmt.Errorf("%w: minimum notice out of range", ErrInvalid)
	}
	if p.MaxDaysAhead == 0 {
		p.MaxDaysAhead = 60
	}
	if p.MaxDaysAhead < 1 || p.MaxDaysAhead > 365 {
		return fmt.Errorf("%w: booking horizon must be 1 to 365 days", ErrInvalid)
	}
	if p.SlotStepMin == 0 {
		p.SlotStepMin = p.DurationMin
	}
	if p.SlotStepMin < 5 || p.SlotStepMin > 480 {
		return fmt.Errorf("%w: slot step must be 5 to 480 minutes", ErrInvalid)
	}
	if p.MaxPerDay < 0 || p.MaxPerDay > 100 {
		return fmt.Errorf("%w: max per day out of range", ErrInvalid)
	}
	if p.Timezone == "" {
		p.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return fmt.Errorf("%w: unknown time zone", ErrInvalid)
	}
	if p.Availability == nil {
		p.Availability = Availability{}
	}
	if err := ValidateAvailability(p.Availability); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if len(p.Questions) > 10 {
		return fmt.Errorf("%w: at most 10 questions", ErrInvalid)
	}
	seen := map[string]bool{}
	for i := range p.Questions {
		q := &p.Questions[i]
		q.Label = strings.TrimSpace(q.Label)
		if q.Label == "" || utf8.RuneCountInString(q.Label) > 200 {
			return fmt.Errorf("%w: each question needs a label (max 200 characters)", ErrInvalid)
		}
		if q.Type != "longtext" {
			q.Type = "text"
		}
		if q.ID == "" || seen[q.ID] {
			q.ID = fmt.Sprintf("q%d", i+1)
		}
		seen[q.ID] = true
	}
	return nil
}

// CreatePage stores a new page for the owner.
func (r *Repository) CreatePage(ctx context.Context, org, owner string, p Page) (Page, error) {
	if err := p.Normalize(); err != nil {
		return Page{}, err
	}
	av, _ := json.Marshal(p.Availability)
	qs, _ := json.Marshal(p.Questions)
	for range 5 {
		slug := Slugify(p.Title) + "-" + randHex(3)
		row := r.pool.QueryRow(ctx, `INSERT INTO adrient.booking_pages
			(org_id, owner_id, slug, title, description, location, duration_min, buffer_before_min, buffer_after_min,
			 min_notice_min, max_days_ahead, slot_step_min, max_per_day, timezone, availability, questions, create_meet, active)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING `+pageCols,
			org, owner, slug, p.Title, p.Description, p.Location, p.DurationMin, p.BufferBeforeMin, p.BufferAfterMin,
			p.MinNoticeMin, p.MaxDaysAhead, p.SlotStepMin, p.MaxPerDay, p.Timezone, av, qs, p.CreateMeet, p.Active)
		out, err := scanPage(row)
		if err == nil {
			return out, nil
		}
		if !strings.Contains(err.Error(), "booking_pages_slug_key") {
			return Page{}, err
		}
	}
	return Page{}, errors.New("could not allocate a unique link")
}

// UpdatePage replaces the editable fields of one of the owner's pages.
func (r *Repository) UpdatePage(ctx context.Context, org, owner, id string, p Page) (Page, error) {
	if err := p.Normalize(); err != nil {
		return Page{}, err
	}
	av, _ := json.Marshal(p.Availability)
	qs, _ := json.Marshal(p.Questions)
	out, err := scanPage(r.pool.QueryRow(ctx, `UPDATE adrient.booking_pages SET title=$4, description=$5, location=$6,
		duration_min=$7, buffer_before_min=$8, buffer_after_min=$9, min_notice_min=$10, max_days_ahead=$11,
		slot_step_min=$12, max_per_day=$13, timezone=$14, availability=$15, questions=$16, create_meet=$17,
		active=$18, updated_at=now()
		WHERE id=$1 AND org_id=$2 AND owner_id=$3 RETURNING `+pageCols,
		id, org, owner, p.Title, p.Description, p.Location, p.DurationMin, p.BufferBeforeMin, p.BufferAfterMin,
		p.MinNoticeMin, p.MaxDaysAhead, p.SlotStepMin, p.MaxPerDay, p.Timezone, av, qs, p.CreateMeet, p.Active))
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	return out, err
}

// DeletePage removes a page (its appointments cascade; calendar events stay).
func (r *Repository) DeletePage(ctx context.Context, org, owner, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.booking_pages WHERE id=$1 AND org_id=$2 AND owner_id=$3`, id, org, owner)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPages returns the owner's pages.
func (r *Repository) ListPages(ctx context.Context, org, owner string) ([]Page, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+pageCols+` FROM adrient.booking_pages
		WHERE org_id=$1 AND owner_id=$2 ORDER BY created_at DESC`, org, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Page{}
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PageBySlug returns an active page by its public slug.
func (r *Repository) PageBySlug(ctx context.Context, slug string) (Page, error) {
	p, err := scanPage(r.pool.QueryRow(ctx, `SELECT `+pageCols+` FROM adrient.booking_pages WHERE slug=$1 AND active`, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return Page{}, ErrNotFound
	}
	return p, err
}

// Rules converts a page into scheduling rules.
func (p Page) Rules() (Rules, error) {
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return Rules{}, err
	}
	return Rules{
		Duration: time.Duration(p.DurationMin) * time.Minute, BufferBefore: time.Duration(p.BufferBeforeMin) * time.Minute,
		BufferAfter: time.Duration(p.BufferAfterMin) * time.Minute, MinNotice: time.Duration(p.MinNoticeMin) * time.Minute,
		MaxDaysAhead: p.MaxDaysAhead, Step: time.Duration(p.SlotStepMin) * time.Minute, MaxPerDay: p.MaxPerDay,
		Location: loc, Avail: p.Availability,
	}, nil
}

// bookedPerDay counts confirmed appointments of the page per local day.
func (r *Repository) bookedPerDay(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, p Page, from, to time.Time) (map[string]int, error) {
	loc, _ := time.LoadLocation(p.Timezone)
	rows, err := q.Query(ctx, `SELECT start_at FROM adrient.booking_appointments
		WHERE page_id=$1 AND status='confirmed' AND start_at >= $2 AND start_at < $3`, p.ID, from.Add(-24*time.Hour), to.Add(24*time.Hour))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s time.Time
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out[s.In(loc).Format("2006-01-02")]++
	}
	return out, rows.Err()
}

// Slots returns the bookable start times of a page in [from, to).
func (r *Repository) Slots(ctx context.Context, p Page, from, to time.Time) ([]time.Time, error) {
	rules, err := p.Rules()
	if err != nil {
		return nil, err
	}
	if to.Sub(from) > 62*24*time.Hour {
		to = from.Add(62 * 24 * time.Hour)
	}
	busy, err := r.busy(ctx, p, from, to)
	if err != nil {
		return nil, err
	}
	booked, err := r.bookedPerDay(ctx, r.pool, p, from, to)
	if err != nil {
		return nil, err
	}
	return ComputeSlots(rules, busy, booked, from, to, r.now()), nil
}

func (r *Repository) busy(ctx context.Context, p Page, from, to time.Time) ([]Span, error) {
	if r.Deps.Busy == nil {
		return nil, nil
	}
	_, email, err := r.ownerInfo(ctx, p)
	if err != nil {
		return nil, err
	}
	// Widen the window so buffers around edge slots are honoured.
	return r.Deps.Busy(ctx, p.OrgID, p.OwnerID, email, from.Add(-6*time.Hour), to.Add(6*time.Hour))
}

func (r *Repository) ownerInfo(ctx context.Context, p Page) (string, string, error) {
	if r.Deps.Owner == nil {
		return "", "", nil
	}
	return r.Deps.Owner(ctx, p.OrgID, p.OwnerID)
}

// BookInput is what a guest submits.
type BookInput struct {
	Start   time.Time
	Name    string
	Email   string
	TZ      string
	Answers map[string]string
}

// Validate checks the guest input against the page's questions.
func (in *BookInput) Validate(p Page) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.TrimSpace(in.Email)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 120 {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	addr, err := mail.ParseAddress(in.Email)
	if err != nil || addr.Address != in.Email || len(in.Email) > 254 {
		return fmt.Errorf("%w: a valid email is required", ErrInvalid)
	}
	if in.TZ == "" {
		in.TZ = "UTC"
	}
	if _, err := time.LoadLocation(in.TZ); err != nil {
		in.TZ = "UTC"
	}
	clean := map[string]string{}
	for _, q := range p.Questions {
		a := strings.TrimSpace(in.Answers[q.ID])
		if q.Required && a == "" {
			return fmt.Errorf("%w: please answer %q", ErrInvalid, q.Label)
		}
		if utf8.RuneCountInString(a) > 2000 {
			return fmt.Errorf("%w: answer to %q is too long", ErrInvalid, q.Label)
		}
		if a != "" {
			clean[q.ID] = a
		}
	}
	in.Answers = clean
	return nil
}

const apptCols = `a.id::text, a.page_id::text, p.title, a.org_id::text, a.owner_id::text, COALESCE(a.event_id::text,''),
	a.start_at, a.end_at, a.guest_name, a.guest_email, a.guest_tz, a.answers, a.meet_code, a.status, a.cancel_token`

func scanAppt(row pgx.Row) (Appointment, error) {
	var a Appointment
	var ans []byte
	err := row.Scan(&a.ID, &a.PageID, &a.PageTitle, &a.OrgID, &a.OwnerID, &a.EventID, &a.StartAt, &a.EndAt,
		&a.GuestName, &a.GuestEmail, &a.GuestTZ, &ans, &a.MeetCode, &a.Status, &a.CancelToken)
	if err != nil {
		return Appointment{}, err
	}
	_ = json.Unmarshal(ans, &a.Answers)
	if a.Answers == nil {
		a.Answers = map[string]string{}
	}
	return a, nil
}

// Book reserves a slot. The owner is locked for the duration so two guests
// cannot take overlapping times, and the slot is re-validated server side.
func (r *Repository) Book(ctx context.Context, p Page, in BookInput) (Appointment, error) {
	if !p.Active {
		return Appointment{}, ErrNotFound
	}
	if err := in.Validate(p); err != nil {
		return Appointment{}, err
	}
	rules, err := p.Rules()
	if err != nil {
		return Appointment{}, err
	}
	start := in.Start.UTC().Truncate(time.Minute)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Appointment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "booking:"+p.OwnerID); err != nil {
		return Appointment{}, err
	}
	// A guest cannot hold two live appointments on the same page at once.
	var dup int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM adrient.booking_appointments
		WHERE page_id=$1 AND lower(guest_email)=lower($2) AND status='confirmed' AND start_at > now()`, p.ID, in.Email).Scan(&dup); err != nil {
		return Appointment{}, err
	}
	if dup >= 3 {
		return Appointment{}, fmt.Errorf("%w: you already have several upcoming bookings on this page", ErrInvalid)
	}
	busy, err := r.busy(ctx, p, start, start.Add(rules.Duration))
	if err != nil {
		return Appointment{}, err
	}
	booked, err := r.bookedPerDay(ctx, tx, p, start, start)
	if err != nil {
		return Appointment{}, err
	}
	if !IsBookable(rules, busy, booked, start, r.now()) {
		return Appointment{}, ErrNotBookable
	}
	end := start.Add(rules.Duration)
	ans, _ := json.Marshal(in.Answers)
	token := randHex(24)
	var apptID string
	err = tx.QueryRow(ctx, `INSERT INTO adrient.booking_appointments
		(page_id, org_id, owner_id, start_at, end_at, guest_name, guest_email, guest_tz, answers, cancel_token)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id::text`,
		p.ID, p.OrgID, p.OwnerID, start, end, in.Name, in.Email, in.TZ, ans, token).Scan(&apptID)
	if err != nil {
		if strings.Contains(err.Error(), "booking_appt_slot_uq") {
			return Appointment{}, ErrSlotTaken
		}
		return Appointment{}, err
	}
	var eventID, meetCode string
	if r.Deps.CreateEvent != nil {
		desc := p.Description
		var b strings.Builder
		if desc != "" {
			b.WriteString(desc + "\n\n")
		}
		fmt.Fprintf(&b, "Booked by %s <%s>", in.Name, in.Email)
		for _, q := range p.Questions {
			if a := in.Answers[q.ID]; a != "" {
				fmt.Fprintf(&b, "\n%s: %s", q.Label, a)
			}
		}
		eventID, meetCode, err = r.Deps.CreateEvent(ctx, p.OrgID, p.OwnerID, EventInput{
			Title: p.Title + " with " + in.Name, Description: b.String(), Location: p.Location,
			Start: start, End: end, GuestEmail: in.Email, Meet: p.CreateMeet,
		})
		if err != nil {
			return Appointment{}, fmt.Errorf("create calendar event: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE adrient.booking_appointments SET event_id=NULLIF($2,'')::uuid, meet_code=$3 WHERE id=$1`,
			apptID, eventID, meetCode); err != nil {
			_ = r.deleteEvent(ctx, p.OrgID, eventID)
			return Appointment{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		_ = r.deleteEvent(ctx, p.OrgID, eventID)
		return Appointment{}, err
	}
	a, err := r.AppointmentByToken(ctx, token)
	if err != nil {
		return Appointment{}, err
	}
	r.sendConfirmation(ctx, p, a)
	return a, nil
}

func (r *Repository) deleteEvent(ctx context.Context, org, id string) error {
	if id == "" || r.Deps.DeleteEvent == nil {
		return nil
	}
	return r.Deps.DeleteEvent(ctx, org, id)
}

// AppointmentByToken finds an appointment by its secret cancel token.
func (r *Repository) AppointmentByToken(ctx context.Context, token string) (Appointment, error) {
	if len(token) < 20 {
		return Appointment{}, ErrNotFound
	}
	a, err := scanAppt(r.pool.QueryRow(ctx, `SELECT `+apptCols+` FROM adrient.booking_appointments a
		JOIN adrient.booking_pages p ON p.id=a.page_id WHERE a.cancel_token=$1`, token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Appointment{}, ErrNotFound
	}
	return a, err
}

// ListAppointments returns the owner's appointments from `from` onward.
func (r *Repository) ListAppointments(ctx context.Context, org, owner string, from time.Time) ([]Appointment, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+apptCols+` FROM adrient.booking_appointments a
		JOIN adrient.booking_pages p ON p.id=a.page_id
		WHERE a.org_id=$1 AND a.owner_id=$2 AND a.start_at >= $3 ORDER BY a.start_at LIMIT 500`, org, owner, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Appointment{}
	for rows.Next() {
		a, err := scanAppt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Cancel cancels an appointment (by the guest with the token, or the owner by id)
// and frees the slot and the calendar event.
func (r *Repository) Cancel(ctx context.Context, a Appointment, byOwner bool) error {
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.booking_appointments SET status='cancelled' WHERE id=$1 AND status='confirmed'`, a.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_ = r.deleteEvent(ctx, a.OrgID, a.EventID)
	r.sendCancellation(ctx, a, byOwner)
	return nil
}

// OwnerAppointment loads one of the owner's appointments by id.
func (r *Repository) OwnerAppointment(ctx context.Context, org, owner, id string) (Appointment, error) {
	a, err := scanAppt(r.pool.QueryRow(ctx, `SELECT `+apptCols+` FROM adrient.booking_appointments a
		JOIN adrient.booking_pages p ON p.id=a.page_id WHERE a.id=$1 AND a.org_id=$2 AND a.owner_id=$3`, id, org, owner))
	if errors.Is(err, pgx.ErrNoRows) {
		return Appointment{}, ErrNotFound
	}
	return a, err
}
