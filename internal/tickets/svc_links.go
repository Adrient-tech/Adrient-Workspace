package tickets

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// linkRecord stores a link between a ticket and a record of another app (shown in the ticket and
// in the other app through the shared record_links table).
func (r *Repository) linkRecord(ctx context.Context, org, ticketID, toApp, toType, toID, label, user string) {
	_, _ = r.pool.Exec(ctx, `INSERT INTO adrient.record_links (org_id, from_app, from_type, from_id, to_app, to_type, to_id, label, created_by)
		VALUES ($1,'tickets','ticket',$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, org, ticketID, toApp, toType, toID, label, nullUUID(user))
}

func titleOf(text, fallback string) string {
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		l = strings.Join(strings.Fields(l), " ")
		if l != "" {
			if len(l) > 120 {
				l = l[:117] + "..."
			}
			return l
		}
	}
	return fallback
}

// FromSource files a ticket from a Mail message the caller owns or a Chat message in a channel the
// caller belongs to. projectID "" picks the oldest project of the organisation.
func (r *Repository) FromSource(ctx context.Context, org, user, source, messageID, projectID string) (Ticket, error) {
	if projectID == "" {
		if err := r.pool.QueryRow(ctx, `SELECT id::text FROM adrient.ticket_projects WHERE org_id=$1 ORDER BY created_at LIMIT 1`, org).Scan(&projectID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Ticket{}, fmt.Errorf("%w: create a help desk project in Tickets first", ErrInvalid)
			}
			return Ticket{}, err
		}
	} else if err := r.projectInOrg(ctx, org, projectID); err != nil {
		return Ticket{}, err
	}
	var title, body, name, email, ref string
	var requesterUser *string
	switch source {
	case "mail":
		var subject, from, fromName, owner string
		var to []byte
		err := r.pool.QueryRow(ctx, `SELECT subject, body, from_addr, from_name, to_addrs, owner_id::text FROM adrient.mail_messages WHERE id::text=$1 AND org_id=$2 AND owner_id::text=$3`,
			messageID, org, user).Scan(&subject, &body, &from, &fromName, &to, &owner)
		if errors.Is(err, pgx.ErrNoRows) {
			return Ticket{}, ErrNotFound
		}
		if err != nil {
			return Ticket{}, err
		}
		title, name, email = titleOf(subject, "Mail request"), fromName, strings.ToLower(from)
		var self string
		_ = r.pool.QueryRow(ctx, `SELECT lower(email) FROM adrient.users WHERE id::text=$1`, user).Scan(&self)
		if email == self || email == "" { // a sent message: the other side is the requester
			if _, a, ok := ParseAddress(firstJSONString(to)); ok {
				email, name = a, ""
			}
		}
		ref = messageID
	case "chat":
		var senderID, senderName string
		err := r.pool.QueryRow(ctx, `SELECT m.body, m.sender_id::text, m.sender_name FROM adrient.chat_messages m JOIN adrient.chat_channels c ON c.id=m.channel_id
			WHERE m.id::text=$1 AND m.org_id=$2 AND c.member_ids ? $3`, messageID, org, user).Scan(&body, &senderID, &senderName)
		if errors.Is(err, pgx.ErrNoRows) {
			return Ticket{}, ErrNotFound
		}
		if err != nil {
			return Ticket{}, err
		}
		title, name = titleOf(body, "Chat request"), senderName
		_ = r.pool.QueryRow(ctx, `SELECT lower(email) FROM adrient.users WHERE id::text=$1 AND org_id=$2`, senderID, org).Scan(&email)
		requesterUser = &senderID
		ref = messageID
	default:
		return Ticket{}, fmt.Errorf("%w: source must be mail or chat", ErrInvalid)
	}
	if len(body) > 20000 {
		body = body[:20000]
	}
	t, err := r.CreateTicket(ctx, org, projectID, title, body, "normal", requesterUser, name, email, source)
	if err != nil {
		return t, err
	}
	r.linkRecord(ctx, org, t.ID, source, "message", ref, "From "+source, user)
	return t, nil
}

func firstJSONString(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	if i := strings.Index(s, ","); i >= 0 {
		s = s[:i]
	}
	return strings.Trim(s, `" `)
}

// CreateIssue files an issue in Projects for a ticket and links both ways. Deps.CreateIssue is nil
// when Projects is not available.
func (r *Repository) CreateIssueFor(ctx context.Context, org, user, ticketID, teamID string) (id, label string, err error) {
	if r.Deps.CreateIssue == nil {
		return "", "", fmt.Errorf("%w: Projects is not available", ErrInvalid)
	}
	t, err := r.GetDesk(ctx, org, ticketID)
	if err != nil {
		return "", "", err
	}
	id, label, err = r.Deps.CreateIssue(ctx, org, user, teamID, fmt.Sprintf("%s %s", t.Ref, t.Title), "From ticket "+t.Ref+" ("+t.RequesterName+").\n\n"+t.Body)
	if err != nil {
		return "", "", err
	}
	r.linkRecord(ctx, org, ticketID, "projects", "issue", id, "Issue "+label, user)
	r.RecordEvent(ctx, org, ticketID, "issue", map[string]any{"issue": label})
	return id, label, nil
}
