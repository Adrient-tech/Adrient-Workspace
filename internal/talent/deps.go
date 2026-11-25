package talent

import (
	"context"
	"io"
	"time"
)

// Deps are the other modules' capabilities Talent uses. Every field is
// optional; a missing one makes the dependent feature report "not available"
// instead of pretending to work.
type Deps struct {
	// Audit records an audit entry.
	Audit func(ctx context.Context, a Actor, action, rtype, rid string)
	// Mail sends one transactional email.
	Mail func(ctx context.Context, to []string, subject, html, text string) error
	// Calendar creates a calendar event with a Meet link.
	Calendar Calendar
	// Tasks creates a task for a user in the Tasks app.
	Tasks func(ctx context.Context, org, assigneeID, title, notes string, due *time.Time) (string, error)
	// Resumes stores resume files in Drive.
	Resumes ResumeStore
	// Now is a clock for tests.
	Now func() time.Time
}

// Calendar creates and removes interview events.
type Calendar interface {
	// CreateMeeting creates an event owned by ownerID with a Meet room and
	// returns the event id and the meet code.
	CreateMeeting(ctx context.Context, org, ownerID, title, desc string, start, end time.Time, attendees []string) (eventID, meetCode string, err error)
	DeleteEvent(ctx context.Context, org, eventID string) error
}

// ResumeStore keeps resume files in the Drive blob store and registers them
// as Drive files owned by a user.
type ResumeStore interface {
	// Save stores data and returns the Drive file id, blob key and the folder id used.
	Save(ctx context.Context, org, ownerID, folderID, name, mime string, data []byte) (fileID, key, folder string, err error)
	Open(ctx context.Context, key string) (io.ReadCloser, string, int64, error)
}

// PDFBlobs stores generated offer PDFs.
type PDFBlobs interface {
	Put(ctx context.Context, key, mimeType string, size int64, body io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, string, int64, error)
}

func (r *Repository) now() time.Time {
	if r.Deps.Now != nil {
		return r.Deps.Now()
	}
	return time.Now()
}
