// Meeting transcript: participants' speech-to-text segments, stored while the host has transcription on.
// The transcript is a hook for summaries and action items (the assistant platform reads it through Segments).
package meet

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Segment is one finished utterance.
type Segment struct {
	Speaker string    `json:"speaker"`
	Text    string    `json:"text"`
	At      time.Time `json:"at"`
}

// MaxSegmentRunes bounds one utterance.
const MaxSegmentRunes = 1000

// maxSegmentsPerRoom bounds the stored transcript of one room.
const maxSegmentsPerRoom = 20000

// CleanSegment trims and validates an utterance. ok is false for empty text.
func CleanSegment(s string) (string, bool) {
	s = strings.ToValidUTF8(strings.Join(strings.Fields(s), " "), "")
	if s == "" {
		return "", false
	}
	if utf8.RuneCountInString(s) > MaxSegmentRunes {
		s = string([]rune(s)[:MaxSegmentRunes])
	}
	return s, true
}

// AddSegment stores one utterance. It returns false when the room is full.
func (r *Repository) AddSegment(ctx context.Context, org, room, user, speaker, text string) (bool, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM adrient.meet_transcript_segments WHERE room_id=$1 AND org_id=$2`, room, org).Scan(&n); err != nil {
		return false, err
	}
	if n >= maxSegmentsPerRoom {
		return false, nil
	}
	if utf8.RuneCountInString(speaker) > 80 {
		speaker = string([]rune(speaker)[:80])
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO adrient.meet_transcript_segments (org_id, room_id, user_id, speaker, body) VALUES ($1,$2,$3,$4,$5)`,
		org, room, user, speaker, text)
	return err == nil, err
}

// Segments returns the transcript of a room in order.
func (r *Repository) Segments(ctx context.Context, org, room string) ([]Segment, error) {
	rows, err := r.pool.Query(ctx, `SELECT speaker, body, at FROM adrient.meet_transcript_segments WHERE room_id=$1 AND org_id=$2 ORDER BY id LIMIT $3`, room, org, maxSegmentsPerRoom)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Segment{}
	for rows.Next() {
		var s Segment
		if err := rows.Scan(&s.Speaker, &s.Text, &s.At); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ClearSegments deletes a room's transcript.
func (r *Repository) ClearSegments(ctx context.Context, org, room string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM adrient.meet_transcript_segments WHERE room_id=$1 AND org_id=$2`, room, org)
	return err
}

// TranscriptText renders segments as plain text: "[14:03] Maya: hello".
// Consecutive segments by one speaker are kept as separate lines so timing stays visible.
func TranscriptText(title string, segs []Segment, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Transcript: %s\n\n", title)
	if len(segs) == 0 {
		b.WriteString("(no speech was recorded)\n")
	}
	for _, s := range segs {
		fmt.Fprintf(&b, "[%s] %s: %s\n", s.At.In(loc).Format("15:04"), s.Speaker, s.Text)
	}
	return b.String()
}
