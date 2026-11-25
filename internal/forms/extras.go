package forms

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// ErrInvalid marks bad input.
var ErrInvalid = errors.New("invalid input")

// ErrMailOff means no mail sender is configured.
var ErrMailOff = errors.New("email is not set up on this server")

// MaxSendRecipients caps one send.
const MaxSendRecipients = 200

// Theme and notification settings of a form.
type Extras struct {
	ThemeColor  string `json:"theme_color"`
	ThemeBG     string `json:"theme_bg"`
	NotifyOwner bool   `json:"notify_owner"`
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Normalize validates colours ("" keeps the default look).
func (e *Extras) Normalize() error {
	e.ThemeColor, e.ThemeBG = strings.TrimSpace(e.ThemeColor), strings.TrimSpace(e.ThemeBG)
	for _, c := range []string{e.ThemeColor, e.ThemeBG} {
		if c != "" && !hexColor.MatchString(c) {
			return fmt.Errorf("%w: colours must look like #126C5A", ErrInvalid)
		}
	}
	return nil
}

// CleanEmails validates, lower-cases and de-duplicates addresses.
func CleanEmails(in []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		a, err := mail.ParseAddress(s)
		if err != nil || a.Address != s || len(s) > 254 {
			return nil, fmt.Errorf("%w: %q is not a valid email address", ErrInvalid, s)
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if len(out) > MaxSendRecipients {
		return nil, fmt.Errorf("%w: at most %d recipients at a time", ErrInvalid, MaxSendRecipients)
	}
	return out, nil
}

// InviteMessage builds the subject, text and HTML of a form invitation.
func InviteMessage(title, sender, note, link string) (subject, text, htm string) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Untitled form"
	}
	subject = sender + " invited you to fill out: " + title
	note = strings.TrimSpace(note)
	var t, h strings.Builder
	if note != "" {
		t.WriteString(note + "\n\n")
		h.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(note), "\n", "<br>") + "</p>")
	}
	fmt.Fprintf(&t, "Fill out the form \"%s\": %s\n", title, link)
	fmt.Fprintf(&h, "<p><a href=\"%s\">Fill out the form &ldquo;%s&rdquo;</a></p>", html.EscapeString(link), html.EscapeString(title))
	return subject, t.String(), h.String()
}

// FormOwner returns the owner id and title of a form in the org.
func (r *Repository) formMeta(ctx context.Context, org, id string) (owner, title string, err error) {
	err = r.pool.QueryRow(ctx, `SELECT owner_id::text, title FROM adrient.forms WHERE id=$1 AND org_id=$2 AND trashed_at IS NULL`, id, org).Scan(&owner, &title)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return
}

// GetExtras returns a form's theme and settings. Notification settings are
// only revealed to the owner.
func (r *Repository) GetExtras(ctx context.Context, org, user, id string) (Extras, error) {
	owner, _, err := r.formMeta(ctx, org, id)
	if err != nil {
		return Extras{}, err
	}
	var e Extras
	err = r.pool.QueryRow(ctx, `SELECT theme_color, theme_bg, notify_owner FROM adrient.form_extras WHERE form_id=$1`, id).Scan(&e.ThemeColor, &e.ThemeBG, &e.NotifyOwner)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Extras{}, err
	}
	if owner != user {
		e.NotifyOwner = false
	}
	return e, nil
}

// SaveExtras stores theme and notification settings (owner only).
func (r *Repository) SaveExtras(ctx context.Context, org, user, id string, e Extras) error {
	if err := e.Normalize(); err != nil {
		return err
	}
	owner, _, err := r.formMeta(ctx, org, id)
	if err != nil {
		return err
	}
	if owner != user {
		return fmt.Errorf("%w: only the form owner can change this", ErrInvalid)
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO adrient.form_extras (form_id, org_id, theme_color, theme_bg, notify_owner)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (form_id) DO UPDATE SET theme_color=$3, theme_bg=$4,
		notify_owner=$5, notified_at = CASE WHEN adrient.form_extras.notify_owner THEN adrient.form_extras.notified_at ELSE now() END, updated_at=now()`,
		id, org, e.ThemeColor, e.ThemeBG, e.NotifyOwner)
	return err
}

// GroupEmails returns the email addresses of the members of a contact group
// owned by the user.
func (r *Repository) GroupEmails(ctx context.Context, org, user, groupID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT c.emails->>0 FROM adrient.contact_group_members m
		JOIN adrient.contact_groups g ON g.id=m.group_id JOIN adrient.contacts c ON c.id=m.contact_id
		WHERE g.id=$1 AND g.org_id=$2 AND g.owner_user_id=$3 AND c.trashed_at IS NULL AND COALESCE(c.emails->>0,'') <> ''`, groupID, org, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SendDeps carries the mail sender.
type SendDeps struct {
	Mail    func(ctx context.Context, to []string, subject, htmlBody, text string) error
	BaseURL string
}

// SendInvites emails the form link to people and/or a contact group (owner only).
func (r *Repository) SendInvites(ctx context.Context, d SendDeps, org, user, senderName, id string, emails []string, groupID, note string) (int, error) {
	if d.Mail == nil || d.BaseURL == "" {
		return 0, ErrMailOff
	}
	owner, title, err := r.formMeta(ctx, org, id)
	if err != nil {
		return 0, err
	}
	if owner != user {
		return 0, fmt.Errorf("%w: only the form owner can send it", ErrInvalid)
	}
	if utf8.RuneCountInString(note) > 2000 {
		return 0, fmt.Errorf("%w: the message is too long", ErrInvalid)
	}
	all := append([]string{}, emails...)
	if groupID != "" {
		g, err := r.GroupEmails(ctx, org, user, groupID)
		if err != nil {
			return 0, err
		}
		all = append(all, g...)
	}
	to, err := CleanEmails(all)
	if err != nil {
		return 0, err
	}
	if len(to) == 0 {
		return 0, fmt.Errorf("%w: add at least one recipient", ErrInvalid)
	}
	var recent int
	_ = r.pool.QueryRow(ctx, `SELECT COALESCE(sum(recipients),0) FROM adrient.form_sends WHERE form_id=$1 AND created_at > now() - interval '1 day'`, id).Scan(&recent)
	if recent+len(to) > 500 {
		return 0, fmt.Errorf("%w: at most 500 invitations per form per day", ErrInvalid)
	}
	if senderName == "" {
		senderName = "A colleague"
	}
	subject, text, htm := InviteMessage(title, senderName, note, d.BaseURL+"/forms/d/"+id+"/viewform")
	sent := 0
	for _, addr := range to { // one message each, so recipients do not see each other
		if err := d.Mail(ctx, []string{addr}, subject, htm, text); err == nil {
			sent++
		}
	}
	if sent == 0 {
		return 0, errors.New("the mail server refused every message")
	}
	_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.form_sends (form_id, org_id, sent_by, recipients) VALUES ($1,$2,$3,$4)`, id, org, user, sent)
	return sent, nil
}

// Notifier delivers a notification.
type Notifier func(ctx context.Context, org, user, title, body, target string) error

// RunNotifications tells owners about new responses on forms that ask for it.
// The cut-off moves forward first, so a response is announced only once.
func (r *Repository) RunNotifications(ctx context.Context, now time.Time, notify Notifier) (int, error) {
	rows, err := r.pool.Query(ctx, `SELECT x.form_id::text, x.org_id::text, f.owner_id::text, f.title, x.notified_at
		FROM adrient.form_extras x JOIN adrient.forms f ON f.id=x.form_id WHERE x.notify_owner AND f.trashed_at IS NULL LIMIT 2000`)
	if err != nil {
		return 0, err
	}
	type item struct {
		id, org, owner, title string
		since                 time.Time
	}
	var list []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.org, &it.owner, &it.title, &it.since); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, it)
	}
	rows.Close()
	sent := 0
	for _, it := range list {
		var n int
		if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.form_responses WHERE form_id=$1 AND created_at > $2 AND created_at <= $3`, it.id, it.since, now).Scan(&n); err != nil || n == 0 {
			continue
		}
		tag, err := r.pool.Exec(ctx, `UPDATE adrient.form_extras SET notified_at=$2 WHERE form_id=$1 AND notified_at=$3`, it.id, now, it.since)
		if err != nil || tag.RowsAffected() == 0 {
			continue
		}
		body := fmt.Sprintf("%d new response", n)
		if n != 1 {
			body += "s"
		}
		if notify != nil && notify(ctx, it.org, it.owner, "New response: "+it.title, body, "/forms/d/"+it.id+"/responses") == nil {
			sent++
		}
	}
	return sent, nil
}
