package tickets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// PortalToken returns (creating when needed) the requester's tracking token for a ticket.
func (r *Repository) PortalToken(ctx context.Context, ticketID string) string {
	var tok *string
	if err := r.pool.QueryRow(ctx, `SELECT portal_token FROM adrient.tickets WHERE id=$1`, ticketID).Scan(&tok); err != nil {
		return ""
	}
	if tok != nil && *tok != "" {
		return *tok
	}
	raw := make([]byte, 18)
	_, _ = rand.Read(raw)
	n := "tk_" + hex.EncodeToString(raw)
	if _, err := r.pool.Exec(ctx, `UPDATE adrient.tickets SET portal_token=$2 WHERE id=$1 AND portal_token IS NULL`, ticketID, n); err != nil {
		return ""
	}
	return n
}

// TrackComment is a public message on a ticket.
type TrackComment struct {
	Author    string    `json:"author"`
	Agent     bool      `json:"agent"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// TrackView is what a requester sees of their own ticket.
type TrackView struct {
	Ref       string         `json:"ref"`
	Title     string         `json:"title"`
	Status    string         `json:"status"`
	Solved    bool           `json:"solved"`
	Priority  string         `json:"priority"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Comments  []TrackComment `json:"comments"`
	CSAT      string         `json:"csat_token,omitempty"`
}

type trackRef struct {
	id, org, project string
	requester        string
}

func (r *Repository) trackRef(ctx context.Context, token string) (trackRef, error) {
	var t trackRef
	if len(token) < 10 || len(token) > 80 {
		return t, ErrNotFound
	}
	err := r.pool.QueryRow(ctx, `SELECT id::text, org_id::text, project_id::text, requester_name FROM adrient.tickets WHERE portal_token=$1`, token).Scan(&t.id, &t.org, &t.project, &t.requester)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// TrackByToken returns the requester's view of a ticket.
func (r *Repository) TrackByToken(ctx context.Context, token string) (TrackView, error) {
	ref, err := r.trackRef(ctx, token)
	if err != nil {
		return TrackView{}, err
	}
	d, err := r.GetDesk(ctx, ref.org, ref.id)
	if err != nil {
		return TrackView{}, err
	}
	defs, _ := r.StatusDefs(ctx, ref.org, ref.project)
	cat := CategoryOf(defs, d.Status)
	v := TrackView{Ref: d.Ref, Title: d.Title, Status: d.Status, Solved: cat == "solved" || cat == "closed", Priority: d.Priority, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, Comments: []TrackComment{}}
	rows, err := r.pool.Query(ctx, `SELECT author_name, author_user_id IS NOT NULL, body, created_at FROM adrient.ticket_comments WHERE ticket_id=$1 AND NOT is_internal ORDER BY created_at`, ref.id)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var c TrackComment
		if err := rows.Scan(&c.Author, &c.Agent, &c.Body, &c.CreatedAt); err != nil {
			return v, err
		}
		v.Comments = append(v.Comments, c)
	}
	if v.Solved {
		if tok, err := r.EnsureCSAT(ctx, ref.org, ref.id); err == nil {
			v.CSAT = tok
		}
	}
	return v, rows.Err()
}

// TrackReply adds a requester message; it reopens a solved ticket.
func (r *Repository) TrackReply(ctx context.Context, token, body string) error {
	ref, err := r.trackRef(ctx, token)
	if err != nil {
		return err
	}
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 20000 {
		return fmt.Errorf("%w: write a message (up to 20000 characters)", ErrInvalid)
	}
	name := ref.requester
	if name == "" {
		name = "Requester"
	}
	_, err = r.AddComment(ctx, ref.org, ref.id, nil, name, body, false)
	return err
}

// TrackClose lets the requester close their own ticket.
func (r *Repository) TrackClose(ctx context.Context, token string) error {
	ref, err := r.trackRef(ctx, token)
	if err != nil {
		return err
	}
	defs, _ := r.StatusDefs(ctx, ref.org, ref.project)
	closed := "closed"
	for _, d := range defs {
		if d.Category == "closed" {
			closed = d.Key
			break
		}
	}
	_, err = r.UpdateTicket(WithActor(ctx, Actor{Name: "Requester"}), ref.org, ref.id, &closed, nil, nil, nil, nil, false)
	return err
}
