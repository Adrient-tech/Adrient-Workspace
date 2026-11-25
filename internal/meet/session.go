package meet

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Meeting end reasons stored in meet_rooms.ended_reason.
const (
	EndedEmpty      = "empty"       // the last participant left
	EndedHost       = "host_ended"  // the host ended it for everyone
	EndedRoomClosed = "room_closed" // the room was closed
)

// ErrMeetingEnded is returned when someone tries to join a meeting that ended.
var ErrMeetingEnded = errors.New("meeting ended")

// EndInfo describes a meeting that just ended.
type EndInfo struct {
	Room         Room
	StartedAt    time.Time
	EndedAt      time.Time
	DurationS    int
	Participants []string // user ids that took part
}

// Ended reports whether the room's current session is over.
func (r Room) Ended() bool { return r.EndedAt != nil }

// DurationS is the length of the finished session in whole seconds.
func (r Room) DurationS() int {
	if r.SessionStartedAt == nil || r.EndedAt == nil || r.EndedAt.Before(*r.SessionStartedAt) {
		return 0
	}
	return int(r.EndedAt.Sub(*r.SessionStartedAt).Seconds())
}

// BeginSession records a participant joining. The first joiner starts the
// session. It reports false (and changes nothing) when the meeting already
// ended: a new session needs RestartSession.
func (r *Repository) BeginSession(ctx context.Context, roomID, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE adrient.meet_rooms SET
		   session_started_at = COALESCE(session_started_at, now()),
		   participant_ids = CASE WHEN $2 = '' OR $2 = ANY(participant_ids) THEN participant_ids
		                          ELSE array_append(participant_ids, $2) END
		 WHERE id=$1 AND ended_at IS NULL`, roomID, userID)
	if err != nil {
		return false, fmt.Errorf("meet.BeginSession: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// EndSession marks the meeting ended. It returns ok=false when there was no
// running session (never started or already ended), so a second trigger for
// the same end (host ends, then the room empties) does nothing.
func (r *Repository) EndSession(ctx context.Context, roomID, reason string) (EndInfo, bool, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE adrient.meet_rooms SET ended_at=now(), ended_reason=$2
		 WHERE id=$1 AND session_started_at IS NOT NULL AND ended_at IS NULL
		 RETURNING `+columns, roomID, reason)
	room, err := scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return EndInfo{}, false, nil
	}
	if err != nil {
		return EndInfo{}, false, fmt.Errorf("meet.EndSession: %w", err)
	}
	return EndInfo{Room: room, StartedAt: *room.SessionStartedAt, EndedAt: *room.EndedAt,
		DurationS: room.DurationS(), Participants: room.ParticipantIDs}, true, nil
}

// RestartSession reopens an ended meeting for a new session.
func (r *Repository) RestartSession(ctx context.Context, orgID, roomID string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE adrient.meet_rooms SET session_started_at=NULL, ended_at=NULL, ended_reason='', participant_ids='{}'
		 WHERE id=$1 AND org_id=$2`, roomID, orgID)
	if err != nil {
		return fmt.Errorf("meet.RestartSession: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
