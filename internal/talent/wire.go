package talent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/calendar"
	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
	"github.com/Adrient-tech/Adrient-Workspace/internal/email"
	"github.com/Adrient-tech/Adrient-Workspace/internal/eventmeet"
	"github.com/Adrient-tech/Adrient-Workspace/internal/meet"
	"github.com/Adrient-tech/Adrient-Workspace/internal/tasks"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WireConfig carries the other modules a production Repository needs.
type WireConfig struct {
	Pool      *pgxpool.Pool
	Blobs     *drive.Blobs
	Drive     *drive.Repository
	Calendar  *calendar.Repository
	Meet      *meet.Repository
	EventMeet *eventmeet.Repository
	Tasks     *tasks.Repository
	Mail      *email.Sender
}

// NewWired builds a Repository connected to Calendar, Meet, Drive, Tasks and mail.
func NewWired(c WireConfig) *Repository {
	r := NewRepository(c.Pool)
	if c.Calendar != nil && c.Meet != nil && c.EventMeet != nil {
		r.Deps.Calendar = calendarAdapter{cal: c.Calendar, meet: c.Meet, em: c.EventMeet}
	}
	if c.Blobs != nil && c.Drive != nil {
		r.Deps.Resumes = resumeAdapter{blobs: c.Blobs, drive: c.Drive}
	}
	if c.Tasks != nil {
		tr := c.Tasks
		r.Deps.Tasks = func(ctx context.Context, org, assignee, title, notes string, due *time.Time) (string, error) {
			lists, err := tr.ListLists(ctx, org, assignee)
			if err != nil {
				return "", err
			}
			var listID string
			for _, l := range lists {
				if l.Name == "HR tasks" {
					listID = l.ID
					break
				}
			}
			if listID == "" {
				l, err := tr.CreateList(ctx, org, assignee, tasks.ListFields{Name: "HR tasks"})
				if err != nil {
					return "", err
				}
				listID = l.ID
			}
			t, err := tr.CreateTask(ctx, org, listID, assignee, tasks.TaskFields{Title: title, Notes: notes, DueAt: due})
			if err != nil {
				return "", err
			}
			return t.ID, nil
		}
	}
	if c.Mail != nil && c.Mail.Configured() {
		m := c.Mail
		r.Deps.Mail = func(ctx context.Context, to []string, subject, html, text string) error {
			return m.Send(ctx, email.Message{To: to, Subject: subject, HTML: html, Text: text})
		}
	}
	return r
}

type calendarAdapter struct {
	cal  *calendar.Repository
	meet *meet.Repository
	em   *eventmeet.Repository
}

func (c calendarAdapter) CreateMeeting(ctx context.Context, org, owner, title, desc string, start, end time.Time, attendees []string) (string, string, error) {
	room, err := c.meet.Create(ctx, org, owner, title)
	if err != nil {
		return "", "", err
	}
	ev, err := c.cal.Create(ctx, org, owner, calendar.Fields{Title: title, Description: desc, StartAt: start, EndAt: end,
		ItemType: calendar.ItemTypeEvent, Status: calendar.StatusBusy, Visibility: calendar.VisibilityDefault,
		Reminders: []int32{30}, Attendees: attendees})
	if err != nil {
		return "", "", err
	}
	for _, e := range attendees {
		if _, err := c.cal.AddAttendee(ctx, org, ev.ID, e, false); err != nil {
			return "", "", err
		}
	}
	if err := c.em.Set(ctx, org, ev.ID, room.ID, room.Code); err != nil {
		return "", "", err
	}
	return ev.ID, room.Code, nil
}

func (c calendarAdapter) DeleteEvent(ctx context.Context, org, id string) error {
	return c.cal.Delete(ctx, org, id)
}

type resumeAdapter struct {
	blobs *drive.Blobs
	drive *drive.Repository
}

func (a resumeAdapter) Save(ctx context.Context, org, owner, folder, name, mime string, data []byte) (string, string, string, error) {
	if folder == "" {
		f, err := a.drive.CreateFolder(ctx, org, owner, "", "Recruitment resumes")
		if err != nil {
			return "", "", "", err
		}
		folder = f.ID
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", err
	}
	key := "blobs/talent/" + hex.EncodeToString(b)
	if err := a.blobs.Put(ctx, key, mime, int64(len(data)), bytes.NewReader(data)); err != nil {
		return "", "", "", err
	}
	f, err := a.drive.CreateFile(ctx, org, owner, folder, name, mime, key, int64(len(data)))
	if err != nil {
		// The saved folder may have been deleted; retry once in the owner's root.
		f, err = a.drive.CreateFile(ctx, org, owner, "", name, mime, key, int64(len(data)))
		if err != nil {
			_ = a.blobs.Delete(ctx, key)
			return "", "", "", err
		}
		folder = ""
	}
	return f.ID, key, folder, nil
}

func (a resumeAdapter) Open(ctx context.Context, key string) (io.ReadCloser, string, int64, error) {
	return a.blobs.Get(ctx, key)
}
