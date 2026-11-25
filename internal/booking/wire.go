package booking

import (
	"context"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/calendar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/email"
	"github.com/Adrient-tech/Adrient-Workspace/internal/eventmeet"
	"github.com/Adrient-tech/Adrient-Workspace/internal/meet"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WireConfig carries the modules a production Repository connects to.
type WireConfig struct {
	Pool      *pgxpool.Pool
	Calendar  *calendar.Repository
	Meet      *meet.Repository
	EventMeet *eventmeet.Repository
	Mail      *email.Sender
	BaseURL   string
}

// NewWired builds a Repository connected to Calendar, Meet and mail.
func NewWired(c WireConfig) *Repository {
	r := NewRepository(c.Pool)
	r.Deps.BaseURL = c.BaseURL
	r.Deps.Owner = func(ctx context.Context, org, owner string) (string, string, error) {
		var name, mail string
		err := c.Pool.QueryRow(ctx, `SELECT display_name, email FROM adrient.users WHERE id=$1 AND org_id=$2`, owner, org).Scan(&name, &mail)
		return name, mail, err
	}
	if c.Calendar != nil {
		r.Deps.Busy = func(ctx context.Context, org, owner, ownerEmail string, from, to time.Time) ([]Span, error) {
			list, err := c.Calendar.BusyFor(ctx, org, owner, ownerEmail, from, to)
			if err != nil {
				return nil, err
			}
			out := make([]Span, len(list))
			for i, b := range list {
				out[i] = Span{Start: b.Start, End: b.End}
			}
			return out, nil
		}
		r.Deps.DeleteEvent = func(ctx context.Context, org, id string) error { return c.Calendar.Delete(ctx, org, id) }
		r.Deps.CreateEvent = func(ctx context.Context, org, owner string, in EventInput) (string, string, error) {
			ev, err := c.Calendar.Create(ctx, org, owner, calendar.Fields{Title: in.Title, Description: in.Description,
				Location: in.Location, StartAt: in.Start, EndAt: in.End, ItemType: calendar.ItemTypeEvent,
				Status: calendar.StatusBusy, Visibility: calendar.VisibilityDefault, Reminders: []int32{30},
				Attendees: []string{in.GuestEmail}})
			if err != nil {
				return "", "", err
			}
			if _, err := c.Calendar.AddAttendee(ctx, org, ev.ID, in.GuestEmail, false); err != nil {
				_ = c.Calendar.Delete(ctx, org, ev.ID)
				return "", "", err
			}
			code := ""
			if in.Meet && c.Meet != nil && c.EventMeet != nil {
				if room, err := c.Meet.Create(ctx, org, owner, in.Title); err == nil {
					if err := c.EventMeet.Set(ctx, org, ev.ID, room.ID, room.Code); err == nil {
						code = room.Code
					}
				}
			}
			return ev.ID, code, nil
		}
	}
	if c.Mail != nil && c.Mail.Configured() {
		m := c.Mail
		r.Deps.Mail = func(ctx context.Context, to []string, subject, html, text, icsName string, ics []byte) error {
			msg := email.Message{To: to, Subject: subject, HTML: html, Text: text}
			if len(ics) > 0 {
				msg.Attachments = []email.Attachment{{Filename: icsName, ContentType: "text/calendar", Data: ics}}
			}
			return m.Send(ctx, msg)
		}
	}
	return r
}
