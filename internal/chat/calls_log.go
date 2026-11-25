package chat

import (
	"context"
	"time"
)

// Server-side call history (migration 0135). Rings, answers, declines and
// room ends write here; Chat > Calls reads it.

// ringWindow is how long a ring stays "ringing" before it counts as missed.
const ringWindow = 45 * time.Second

// CallEntry is one call as seen by one user.
type CallEntry struct {
	ID         string   `json:"id"`
	Code       string   `json:"code"`
	Direction  string   `json:"direction"` // in | out
	Status     string   `json:"status"`    // answered | missed | declined | ringing
	Video      bool     `json:"video"`
	Title      string   `json:"title"`
	CallerID   string   `json:"caller_id"`
	PeerIDs    []string `json:"peer_ids"` // everyone else on the call
	StartedAt  string   `json:"started_at"`
	AnsweredAt string   `json:"answered_at"`
	EndedAt    string   `json:"ended_at"`
	DurationS  int      `json:"duration_s"`
}

// LogRing records a call and its rung parties. A repeat ring for the same
// code by the same caller within a day extends the existing call.
func (s *Store) LogRing(ctx context.Context, orgID, callerID, code, title string, video bool, userIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx,
		`SELECT id::text FROM adrient.chat_calls
		 WHERE org_id=$1 AND code=$2 AND caller_id=$3 AND started_at > now() - interval '1 day'
		 ORDER BY started_at DESC LIMIT 1`, orgID, code, callerID).Scan(&id)
	if err != nil {
		if err := tx.QueryRow(ctx,
			`INSERT INTO adrient.chat_calls (org_id, code, caller_id, title, video)
			 VALUES ($1,$2,$3,$4,$5) RETURNING id::text`, orgID, code, callerID, title, video).Scan(&id); err != nil {
			return err
		}
	}
	for _, u := range userIDs {
		if _, err := tx.Exec(ctx,
			`INSERT INTO adrient.chat_call_parties (call_id, user_id) VALUES ($1,$2)
			 ON CONFLICT (call_id, user_id) DO NOTHING`, id, u); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// LogRespond records a party's answer. The callee answers (to = caller);
// the caller cancelling (to = callee) marks the callee as missed.
func (s *Store) LogRespond(ctx context.Context, orgID, uid, to, code, answer string) error {
	switch answer {
	case "accepted", "declined":
		// callee (uid) answering the call started by `to`
		_, err := s.pool.Exec(ctx,
			`WITH c AS (
			   SELECT id FROM adrient.chat_calls
			   WHERE org_id=$1 AND code=$2 AND caller_id=$3 ORDER BY started_at DESC LIMIT 1)
			 UPDATE adrient.chat_call_parties p SET status=$5, responded_at=now()
			 FROM c WHERE p.call_id=c.id AND p.user_id=$4 AND p.status='ringing'`,
			orgID, code, to, uid, answer)
		if err != nil {
			return err
		}
		if answer == "accepted" {
			_, err = s.pool.Exec(ctx,
				`UPDATE adrient.chat_calls SET answered_at=COALESCE(answered_at, now())
				 WHERE id=(SELECT id FROM adrient.chat_calls WHERE org_id=$1 AND code=$2 AND caller_id=$3
				           ORDER BY started_at DESC LIMIT 1)`, orgID, code, to)
		}
		return err
	default: // cancelled by the caller before the callee answered
		_, err := s.pool.Exec(ctx,
			`WITH c AS (
			   SELECT id FROM adrient.chat_calls
			   WHERE org_id=$1 AND code=$2 AND caller_id=$3 ORDER BY started_at DESC LIMIT 1)
			 UPDATE adrient.chat_call_parties p SET status='missed', responded_at=now()
			 FROM c WHERE p.call_id=c.id AND p.user_id=$4 AND p.status='ringing'`,
			orgID, code, uid, to)
		return err
	}
}

// LogEnd stamps the end of the latest call with this code that the user took
// part in (caller or party). Each leaver moves it later, so it settles on the
// last person out.
func (s *Store) LogEnd(ctx context.Context, orgID, uid, code string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE adrient.chat_calls SET ended_at=now()
		 WHERE id=(SELECT c.id FROM adrient.chat_calls c
		           WHERE c.org_id=$1 AND c.code=$2 AND (c.caller_id=$3 OR EXISTS
		             (SELECT 1 FROM adrient.chat_call_parties p WHERE p.call_id=c.id AND p.user_id=$3))
		           ORDER BY c.started_at DESC LIMIT 1)`, orgID, code, uid)
	return err
}

// CallHistory lists the user's calls, newest first.
func (s *Store) CallHistory(ctx context.Context, orgID, uid string, limit int) ([]CallEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 60
	}
	rows, err := s.pool.Query(ctx,
		`SELECT c.id::text, c.code, c.caller_id::text, c.title, c.video, c.started_at, c.answered_at, c.ended_at,
		        COALESCE((SELECT p.status FROM adrient.chat_call_parties p WHERE p.call_id=c.id AND p.user_id=$2), ''),
		        COALESCE((SELECT array_agg(p.user_id::text) FROM adrient.chat_call_parties p WHERE p.call_id=c.id AND p.user_id<>$2), '{}'),
		        COALESCE((SELECT array_agg(p.status) FROM adrient.chat_call_parties p WHERE p.call_id=c.id), '{}')
		 FROM adrient.chat_calls c
		 WHERE c.org_id=$1 AND (c.caller_id=$2 OR EXISTS
		   (SELECT 1 FROM adrient.chat_call_parties p WHERE p.call_id=c.id AND p.user_id=$2))
		 ORDER BY c.started_at DESC LIMIT $3`, orgID, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CallEntry{}
	now := time.Now()
	for rows.Next() {
		var e CallEntry
		var started time.Time
		var answered, ended *time.Time
		var mine string
		var peers, statuses []string
		if err := rows.Scan(&e.ID, &e.Code, &e.CallerID, &e.Title, &e.Video, &started, &answered, &ended, &mine, &peers, &statuses); err != nil {
			return nil, err
		}
		e.StartedAt = started.UTC().Format(time.RFC3339)
		e.AnsweredAt, e.EndedAt = ts(answered), ts(ended)
		fresh := now.Sub(started) < ringWindow
		if e.CallerID == uid {
			e.Direction = "out"
			e.PeerIDs = peers
			e.Status = outStatus(statuses, fresh)
		} else {
			e.Direction = "in"
			e.PeerIDs = append([]string{e.CallerID}, peers...)
			e.Status = inStatus(mine, fresh)
		}
		if answered != nil && ended != nil && ended.After(*answered) {
			e.DurationS = int(ended.Sub(*answered).Seconds())
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func inStatus(mine string, fresh bool) string {
	switch mine {
	case "accepted":
		return "answered"
	case "declined":
		return "declined"
	case "ringing":
		if fresh {
			return "ringing"
		}
	}
	return "missed"
}

func outStatus(parties []string, fresh bool) string {
	allDeclined, ringing := len(parties) > 0, false
	for _, s := range parties {
		if s == "accepted" {
			return "answered"
		}
		if s != "declined" {
			allDeclined = false
		}
		if s == "ringing" {
			ringing = true
		}
	}
	switch {
	case allDeclined:
		return "declined"
	case ringing && fresh:
		return "ringing"
	}
	return "missed"
}
