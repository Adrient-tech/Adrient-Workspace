package tickets

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Scheduled is a reply or note that will be posted later.
type Scheduled struct {
	ID       string    `json:"id"`
	TicketID string    `json:"ticket_id"`
	Body     string    `json:"body"`
	Internal bool      `json:"internal"`
	Solve    bool      `json:"solve"`
	SendAt   time.Time `json:"send_at"`
	State    string    `json:"state"`
	Error    string    `json:"error,omitempty"`
	UserID   string    `json:"user_id"`
	UserName string    `json:"user_name"`
}

// ScheduleReply queues a reply for a later time (at least a minute ahead, at most 90 days).
func (r *Repository) ScheduleReply(ctx context.Context, org, user, userName, ticketID string, s Scheduled) (Scheduled, error) {
	if _, err := r.GetDesk(ctx, org, ticketID); err != nil {
		return s, err
	}
	s.Body = strings.TrimSpace(s.Body)
	if s.Body == "" || len(s.Body) > 50000 {
		return s, fmt.Errorf("%w: write the message to send (up to 50000 characters)", ErrInvalid)
	}
	now := time.Now()
	if s.SendAt.Before(now.Add(time.Minute)) || s.SendAt.After(now.AddDate(0, 0, 90)) {
		return s, fmt.Errorf("%w: choose a time between one minute and 90 days from now", ErrInvalid)
	}
	var pending int
	_ = r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.ticket_scheduled WHERE ticket_id=$1 AND user_id=$2 AND state='pending'`, ticketID, user).Scan(&pending)
	if pending >= 10 {
		return s, fmt.Errorf("%w: at most 10 scheduled messages per ticket", ErrInvalid)
	}
	err := r.pool.QueryRow(ctx, `INSERT INTO adrient.ticket_scheduled (org_id, ticket_id, user_id, user_name, body, internal, solve, send_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text, state`, org, ticketID, user, userName, s.Body, s.Internal, s.Solve && !s.Internal, s.SendAt).Scan(&s.ID, &s.State)
	s.TicketID, s.UserID, s.UserName = ticketID, user, userName
	r.RecordEvent(ctx, org, ticketID, "scheduled", map[string]any{"at": s.SendAt, "internal": s.Internal})
	return s, err
}

// ListScheduled returns the pending messages of a ticket (everyone sees them, only the author cancels).
func (r *Repository) ListScheduled(ctx context.Context, org, ticketID string) ([]Scheduled, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, ticket_id::text, body, internal, solve, send_at, state, error, user_id::text, user_name
		FROM adrient.ticket_scheduled WHERE org_id=$1 AND ticket_id=$2 AND state IN ('pending','failed') ORDER BY send_at LIMIT 50`, org, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Scheduled{}
	for rows.Next() {
		var s Scheduled
		if err := rows.Scan(&s.ID, &s.TicketID, &s.Body, &s.Internal, &s.Solve, &s.SendAt, &s.State, &s.Error, &s.UserID, &s.UserName); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CancelScheduled cancels a pending message; only its author or an admin may.
func (r *Repository) CancelScheduled(ctx context.Context, org, user string, admin bool, id string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE adrient.ticket_scheduled SET state='cancelled' WHERE org_id=$1 AND id=$2 AND state IN ('pending','failed') AND ($4 OR user_id::text=$3)`, org, id, user, admin)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// sendDueScheduled posts every scheduled message that is due. A message is claimed by moving it to
// "sent" first, so two workers never post it twice.
func (r *Repository) sendDueScheduled(ctx context.Context) int {
	rows, err := r.pool.Query(ctx, `UPDATE adrient.ticket_scheduled SET state='sent' WHERE id IN
		(SELECT id FROM adrient.ticket_scheduled WHERE state='pending' AND send_at <= now() ORDER BY send_at LIMIT 50 FOR UPDATE SKIP LOCKED)
		RETURNING id::text, org_id::text, ticket_id::text, user_id::text, user_name, body, internal, solve`)
	if err != nil {
		slog.Warn("tickets: scheduled send query failed", "err", err)
		return 0
	}
	type due struct {
		id, org, ticket, user, name, body string
		internal, solve                   bool
	}
	var list []due
	for rows.Next() {
		var d due
		if rows.Scan(&d.id, &d.org, &d.ticket, &d.user, &d.name, &d.body, &d.internal, &d.solve) == nil {
			list = append(list, d)
		}
	}
	rows.Close()
	sent := 0
	for _, d := range list {
		uctx := WithActor(ctx, Actor{ID: d.user, Name: d.name})
		fail := func(msg string) {
			_, _ = r.pool.Exec(ctx, `UPDATE adrient.ticket_scheduled SET state='failed', error=$2 WHERE id=$1`, d.id, msg)
			r.notify(WithActor(ctx, Actor{}), d.org, d.user, "A scheduled reply could not be sent", msg, d.ticket)
		}
		if ok, _ := r.memberOf(ctx, d.org, d.user); !ok {
			fail("the author is no longer in the organisation")
			continue
		}
		uid := d.user
		if _, err := r.AddComment(uctx, d.org, d.ticket, &uid, d.name, d.body, d.internal); err != nil {
			fail(err.Error())
			continue
		}
		if d.solve && !d.internal {
			if defs, err := r.StatusDefs(ctx, d.org, r.projectOf(ctx, d.ticket)); err == nil {
				for _, s := range defs {
					if s.Category == "solved" {
						k := s.Key
						if _, err := r.UpdateTicket(uctx, d.org, d.ticket, &k, nil, nil, nil, nil, false); err != nil {
							r.RecordEvent(uctx, d.org, d.ticket, "scheduled", map[string]any{"solve_failed": err.Error()})
						}
						break
					}
				}
			}
		}
		sent++
	}
	return sent
}

func (r *Repository) projectOf(ctx context.Context, ticketID string) string {
	var p string
	_ = r.pool.QueryRow(ctx, `SELECT project_id::text FROM adrient.tickets WHERE id=$1`, ticketID).Scan(&p)
	return p
}

// ---- attachments (Drive) ---------------------------------------------------------

// Attachment is a Drive file attached to a ticket.
type Attachment struct {
	ID     string `json:"id"`
	FileID string `json:"file_id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	By     string `json:"added_by"`
}

// AttachFile records a Drive file on a ticket. The file must exist in the organisation; opening it
// still goes through Drive's own access checks.
func (r *Repository) AttachFile(ctx context.Context, org, user, ticketID, fileID string) (Attachment, error) {
	if _, err := r.GetDesk(ctx, org, ticketID); err != nil {
		return Attachment{}, err
	}
	var a Attachment
	var name string
	var size int64
	err := r.pool.QueryRow(ctx, `SELECT name, size_bytes FROM adrient.drive_files WHERE id::text=$1 AND org_id=$2 AND trashed_at IS NULL AND storage_key IS NOT NULL`, fileID, org).Scan(&name, &size)
	if err != nil {
		return a, ErrNotFound
	}
	err = r.pool.QueryRow(ctx, `INSERT INTO adrient.ticket_attachments (org_id, ticket_id, file_id, name, size, added_by) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
		org, ticketID, fileID, name, size, nullUUID(user)).Scan(&a.ID)
	a.FileID, a.Name, a.Size = fileID, name, size
	r.RecordEvent(ctx, org, ticketID, "attachment", map[string]any{"name": name})
	return a, err
}

// ListAttachments returns a ticket's Drive attachments.
func (r *Repository) ListAttachments(ctx context.Context, org, ticketID string) ([]Attachment, error) {
	rows, err := r.pool.Query(ctx, `SELECT a.id::text, a.file_id, a.name, a.size, COALESCE(u.display_name,'') FROM adrient.ticket_attachments a
		LEFT JOIN adrient.users u ON u.id=a.added_by WHERE a.org_id=$1 AND a.ticket_id=$2 ORDER BY a.created_at LIMIT 100`, org, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Attachment{}
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.FileID, &a.Name, &a.Size, &a.By); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// RemoveAttachment removes the record (the Drive file is untouched).
func (r *Repository) RemoveAttachment(ctx context.Context, org, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM adrient.ticket_attachments WHERE org_id=$1 AND id=$2`, org, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
