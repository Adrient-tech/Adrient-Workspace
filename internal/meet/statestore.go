package meet

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// persistedState is the durable part of a call's room state.
type persistedState struct {
	HostID      string
	Locked      bool
	WaitingRoom bool
	EgressID    string
	Banned      []string
	Admitted    []string
	Denied      []string
	Waiting     []waiter
}

// StateStore persists room state across backend restarts.
type StateStore interface {
	LoadState(ctx context.Context, roomID string) (*persistedState, error)
	SaveState(ctx context.Context, roomID string, p persistedState) error
}

// LoadState returns the saved state of a room, or nil when none was saved.
func (r *Repository) LoadState(ctx context.Context, roomID string) (*persistedState, error) {
	var p persistedState
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(host_id::text,''), locked, waiting_room, egress_id
		   FROM adrient.meet_room_state WHERE room_id=$1::uuid`, roomID).
		Scan(&p.HostID, &p.Locked, &p.WaitingRoom, &p.EgressID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("meet.LoadState: %w", err)
	}
	rows, err := r.pool.Query(ctx, `SELECT kind, subject FROM adrient.meet_room_members WHERE room_id=$1::uuid`, roomID)
	if err != nil {
		return nil, fmt.Errorf("meet.LoadState members: %w", err)
	}
	for rows.Next() {
		var kind, subj string
		if err := rows.Scan(&kind, &subj); err != nil {
			rows.Close()
			return nil, err
		}
		switch kind {
		case "ban":
			p.Banned = append(p.Banned, subj)
		case "admit":
			p.Admitted = append(p.Admitted, subj)
		case "deny":
			p.Denied = append(p.Denied, subj)
		}
	}
	rows.Close()
	wr, err := r.pool.Query(ctx, `SELECT identity, name, since FROM adrient.meet_waiters WHERE room_id=$1::uuid`, roomID)
	if err != nil {
		return nil, fmt.Errorf("meet.LoadState waiters: %w", err)
	}
	defer wr.Close()
	for wr.Next() {
		var w waiter
		if err := wr.Scan(&w.Identity, &w.Name, &w.Since); err != nil {
			return nil, err
		}
		w.LastSeen = time.Now() // grace period: they re-poll within the TTL or drop off
		p.Waiting = append(p.Waiting, w)
	}
	return &p, wr.Err()
}

// SaveState replaces the saved state of a room.
func (r *Repository) SaveState(ctx context.Context, roomID string, p persistedState) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("meet.SaveState: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var host any
	if p.HostID != "" {
		host = p.HostID
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO adrient.meet_room_state (room_id, host_id, locked, waiting_room, egress_id, updated_at)
		 VALUES ($1::uuid, $2::uuid, $3, $4, $5, now())
		 ON CONFLICT (room_id) DO UPDATE SET host_id=EXCLUDED.host_id, locked=EXCLUDED.locked,
		   waiting_room=EXCLUDED.waiting_room, egress_id=EXCLUDED.egress_id, updated_at=now()`,
		roomID, host, p.Locked, p.WaitingRoom, p.EgressID); err != nil {
		return fmt.Errorf("meet.SaveState state: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.meet_room_members WHERE room_id=$1::uuid`, roomID); err != nil {
		return err
	}
	ins := func(kind string, subs []string) error {
		for _, s := range subs {
			if _, err := tx.Exec(ctx,
				`INSERT INTO adrient.meet_room_members (room_id, kind, subject) VALUES ($1::uuid,$2,$3) ON CONFLICT DO NOTHING`,
				roomID, kind, s); err != nil {
				return err
			}
		}
		return nil
	}
	if err := ins("ban", p.Banned); err != nil {
		return err
	}
	if err := ins("admit", p.Admitted); err != nil {
		return err
	}
	if err := ins("deny", p.Denied); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM adrient.meet_waiters WHERE room_id=$1::uuid`, roomID); err != nil {
		return err
	}
	for _, w := range p.Waiting {
		if _, err := tx.Exec(ctx,
			`INSERT INTO adrient.meet_waiters (room_id, identity, name, since) VALUES ($1::uuid,$2,$3,$4) ON CONFLICT DO NOTHING`,
			roomID, w.Identity, w.Name, w.Since); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// DeleteState drops the saved state (meeting ended for everyone).
func (r *Repository) DeleteState(ctx context.Context, roomID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.meet_room_state WHERE room_id=$1::uuid`, roomID)
	if err != nil {
		return err
	}
	if _, err = r.pool.Exec(ctx, `DELETE FROM adrient.meet_room_members WHERE room_id=$1::uuid`, roomID); err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM adrient.meet_waiters WHERE room_id=$1::uuid`, roomID)
	return err
}

// ChannelForCode returns the chat channel whose huddle uses code, or "".
func (r *Repository) ChannelForCode(ctx context.Context, orgID, code string) string {
	var id string
	if err := r.pool.QueryRow(ctx,
		`SELECT id::text FROM adrient.chat_channels WHERE org_id=$1::uuid AND huddle_code=$2 AND NOT archived LIMIT 1`,
		orgID, code).Scan(&id); err != nil {
		return ""
	}
	return id
}
