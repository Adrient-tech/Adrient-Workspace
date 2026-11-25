package booking

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"
)

func icsEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", "")
	return r.Replace(s)
}

// BuildICS renders a single-event iCalendar invitation (RFC 5545).
func BuildICS(a Appointment, p Page, ownerName, ownerEmail, meetURL string, cancelled bool) []byte {
	method, status := "REQUEST", "CONFIRMED"
	if cancelled {
		method, status = "CANCEL", "CANCELLED"
	}
	loc := p.Location
	if meetURL != "" {
		loc = meetURL
	}
	lines := []string{
		"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Adrient Workspace//Booking//EN", "CALSCALE:GREGORIAN", "METHOD:" + method,
		"BEGIN:VEVENT",
		"UID:" + a.ID + "@adrient-booking",
		"DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z"),
		"DTSTART:" + a.StartAt.UTC().Format("20060102T150405Z"),
		"DTEND:" + a.EndAt.UTC().Format("20060102T150405Z"),
		"SUMMARY:" + icsEscape(p.Title),
		"STATUS:" + status,
	}
	if loc != "" {
		lines = append(lines, "LOCATION:"+icsEscape(loc))
	}
	if ownerEmail != "" {
		lines = append(lines, fmt.Sprintf("ORGANIZER;CN=%s:mailto:%s", icsEscape(ownerName), ownerEmail))
	}
	lines = append(lines, fmt.Sprintf("ATTENDEE;CN=%s;RSVP=FALSE:mailto:%s", icsEscape(a.GuestName), a.GuestEmail), "END:VEVENT", "END:VCALENDAR")
	var out []byte
	for _, l := range lines {
		out = append(out, foldICS(l)...)
	}
	return out
}

// foldICS folds a content line at 75 octets, CRLF terminated.
func foldICS(l string) []byte {
	var b strings.Builder
	for len(l) > 75 {
		cut := 75
		for cut > 0 && l[cut]&0xC0 == 0x80 { // do not split a UTF-8 sequence
			cut--
		}
		b.WriteString(l[:cut] + "\r\n ")
		l = l[cut:]
	}
	b.WriteString(l + "\r\n")
	return []byte(b.String())
}

func (r *Repository) meetURL(a Appointment) string {
	if a.MeetCode == "" || r.Deps.BaseURL == "" {
		return ""
	}
	return r.Deps.BaseURL + "/meet/" + a.MeetCode
}

func (r *Repository) manageURL(a Appointment) string {
	if r.Deps.BaseURL == "" {
		return ""
	}
	return r.Deps.BaseURL + "/book/cancel/" + a.CancelToken
}

func whenIn(t time.Time, tz string) string {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("Monday, 2 January 2006, 15:04 MST")
}

func (r *Repository) sendConfirmation(ctx context.Context, p Page, a Appointment) {
	if r.Deps.Mail == nil {
		return
	}
	ownerName, ownerEmail, _ := r.ownerInfo(ctx, p)
	meet := r.meetURL(a)
	ics := BuildICS(a, p, ownerName, ownerEmail, meet, false)
	mk := func(who, tz, link string) (string, string) {
		var t, h strings.Builder
		fmt.Fprintf(&t, "Hi %s,\n\nYour appointment \"%s\" is confirmed.\nWhen: %s\n", who, p.Title, whenIn(a.StartAt, tz))
		fmt.Fprintf(&h, "<p>Hi %s,</p><p>Your appointment <b>%s</b> is confirmed.</p><p><b>When:</b> %s</p>",
			html.EscapeString(who), html.EscapeString(p.Title), html.EscapeString(whenIn(a.StartAt, tz)))
		if ownerName != "" {
			fmt.Fprintf(&t, "With: %s\n", ownerName)
			fmt.Fprintf(&h, "<p><b>With:</b> %s</p>", html.EscapeString(ownerName))
		}
		if p.Location != "" {
			fmt.Fprintf(&t, "Where: %s\n", p.Location)
			fmt.Fprintf(&h, "<p><b>Where:</b> %s</p>", html.EscapeString(p.Location))
		}
		if meet != "" {
			fmt.Fprintf(&t, "Video call: %s\n", meet)
			fmt.Fprintf(&h, `<p><b>Video call:</b> <a href="%s">%s</a></p>`, html.EscapeString(meet), html.EscapeString(meet))
		}
		if link != "" {
			fmt.Fprintf(&t, "\nNeed to cancel? %s\n", link)
			fmt.Fprintf(&h, `<p>Need to cancel? <a href="%s">Cancel this appointment</a>.</p>`, html.EscapeString(link))
		}
		return t.String(), h.String()
	}
	t, h := mk(a.GuestName, a.GuestTZ, r.manageURL(a))
	if err := r.Deps.Mail(ctx, []string{a.GuestEmail}, "Confirmed: "+p.Title, h, t, "invite.ics", ics); err != nil {
		slog.Warn("booking: confirmation mail failed", "err", err)
	}
	if ownerEmail != "" {
		t, h := mk(ownerName, p.Timezone, "")
		t = fmt.Sprintf("%s (%s) booked this time.\n\n", a.GuestName, a.GuestEmail) + t
		h = fmt.Sprintf("<p>%s (%s) booked this time.</p>", html.EscapeString(a.GuestName), html.EscapeString(a.GuestEmail)) + h
		if err := r.Deps.Mail(ctx, []string{ownerEmail}, "New booking: "+a.GuestName+" - "+p.Title, h, t, "invite.ics", ics); err != nil {
			slog.Warn("booking: owner mail failed", "err", err)
		}
	}
}

func (r *Repository) sendCancellation(ctx context.Context, a Appointment, byOwner bool) {
	if r.Deps.Mail == nil {
		return
	}
	p := Page{ID: a.PageID, OrgID: a.OrgID, OwnerID: a.OwnerID, Title: a.PageTitle}
	ownerName, ownerEmail, _ := r.ownerInfo(ctx, p)
	ics := BuildICS(a, p, ownerName, ownerEmail, "", true)
	who := ownerName
	if !byOwner {
		who = a.GuestName
	}
	text := fmt.Sprintf("The appointment \"%s\" on %s was cancelled by %s.\n", a.PageTitle, whenIn(a.StartAt, a.GuestTZ), who)
	htm := "<p>" + html.EscapeString(text) + "</p>"
	to := []string{a.GuestEmail}
	if !byOwner && ownerEmail != "" {
		to = []string{ownerEmail}
	}
	if byOwner && ownerEmail != "" {
		to = []string{a.GuestEmail}
	}
	if err := r.Deps.Mail(ctx, to, "Cancelled: "+a.PageTitle, htm, text, "cancel.ics", ics); err != nil {
		slog.Warn("booking: cancellation mail failed", "err", err)
	}
}
